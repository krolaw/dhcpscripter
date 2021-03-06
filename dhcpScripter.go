package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io/ioutil"
	"log/syslog"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	// Message Types we are interested in
	DHCPDiscover = 1

	// DHCP Options
	DHCPMessageType = 53
	HostName        = 12

	// Default DHCP Port
	DHCPUDPPort = 67

	// Duplicate Packet Timeout
	DuplicatePacketTimeout = 20 * time.Second
)

type config struct {
	Port    int
	SysLog  bool
	FileLog string
	NICs    map[string]NIC // Commands
	/*XMPPServer   string
	XMPPLogin    string
	XMPPPassword string
	XMPPLog      []string*/
}

type NIC struct {
	Name    string
	Cmd     []string // Single Command
	SysLog  *bool    // Override for this nic
	FileLog *string  // Override for this nic
	// XMPPLog *[]string // Override for this 
	// Cmds    [][]string // Multiple Commands (run consecutively)
}

type dhcpMessage struct { // Overkill for this program, but here to learn
	Raw     []byte
	Op      *byte            // 1 = Request, 2 = Reply
	HType   *byte            // 1 = Ethernet
	HLen    *byte            // MAC length, 6 for ethernet
	Hops    *byte            // 
	XId     []byte           // Client transaction id
	Secs    []byte           // Client filled - reflect
	Flags   []byte           // ???
	CIAddr  net.IP           // Requested IP
	YIAddr  net.IP           // Granted IP
	SIAddr  net.IP           // Server IP
	GIAddr  net.IP           // Gateway IP?
	CHAddr  net.HardwareAddr // Client hardware address
	Cookie  []byte
	Options []byte
}

func (dm *dhcpMessage) DecodeOptions() map[byte][]byte {
	opts := dm.Options
	options := make(map[byte][]byte, 10)
	for len(opts) >= 2 {
		size := int(opts[1])
		if len(opts) >= 2+size {
			options[opts[0]] = opts[2 : 2+size]
		}
		opts = opts[2+size:]
	}
	return options
}

func parseDHCP(r []byte) (*dhcpMessage, error) { // Overkill for this program, but here to learn
	if len(r) < 240 {
		return nil, errors.New("Packet not long enough.")
	}

	return &dhcpMessage{
		Raw:    r,
		Op:     &r[0],
		HType:  &r[1],
		HLen:   &r[2],
		Hops:   &r[3],
		XId:    r[4:8],
		Secs:   r[8:10],
		Flags:  r[10:12],
		CIAddr: net.IP(r[12:16]),
		YIAddr: net.IP(r[16:20]),
		SIAddr: net.IP(r[20:24]),
		GIAddr: net.IP(r[24:28]),
		CHAddr: net.HardwareAddr(r[28 : 28+r[2]]), // max endPos 44
		// 192 bytes of zeros BOOTP legacy
		Cookie:  r[236:240],
		Options: r[240:],
	}, nil
}

func main() {
	configFile := flag.String("conf", "dhcps.conf", "Location of config file")
	flag.Parse()

	// Read Config File
	data, err := ioutil.ReadFile(*configFile)
	if err != nil {
		os.Stderr.WriteString("Error: " + err.Error())
		return
	}

	config := &config{} // Empty config struct

	// Load data into config struct
	if err := json.Unmarshal(data, config); err != nil {
		os.Stderr.WriteString("Error:" + err.Error())
		return
	}

	if config.Port < 1 { // If not specified will be zero, so use default port
		config.Port = DHCPUDPPort
	}

	// Listen
	conn, err := net.ListenPacket("udp", ":"+strconv.Itoa(config.Port))
	if err != nil {
		os.Stderr.WriteString("Error:" + err.Error())
		return
	}

	// Make all NICs lower case so matching works
	for k, v := range config.NICs {
		delete(config.NICs, k)
		config.NICs[strings.ToLower(k)] = v
	}

	buffer := make([]byte, 1500)

	lastFire := make(map[string]time.Time, 1) // Some devices send multiple packets in a row

	currentlyExecuting := make(map[string]bool, 1)

	for {
		n, _, err := conn.ReadFrom(buffer)
		if err != nil {
			panic(err.Error())
		}
		//fmt.Println("Packet In:", buffer[:n])
		dhcpPacket, err := parseDHCP(buffer[:n])
		if err != nil {
			continue
		}
		nicAddr := dhcpPacket.CHAddr.String()

		now := time.Now()
		expire := now.Add(-DuplicatePacketTimeout)
		for i, t := range lastFire {
			if t.Before(expire) {
				delete(lastFire, i)
			}
		}

		if _, ok := lastFire[nicAddr]; ok { // Filter duplicate packet
			continue
		}

		nic, ok := config.NICs[nicAddr]
		if !ok {
			nic = config.NICs["default"]
		}

		options := dhcpPacket.DecodeOptions()

		if msgType := options[DHCPMessageType]; len(msgType) != 1 || msgType[0] != DHCPDiscover {
			continue
		}

		hostname := string(options[HostName])
		if hostname == "" {
			hostname = "*"
		}

		if (nic.SysLog != nil && *nic.SysLog == true) || (nic.SysLog == nil && config.SysLog == true) {
			if w, err := syslog.New(syslog.LOG_NOTICE, "DHCPScripter"); err == nil {
				if err = w.Notice(nicAddr + " (" + hostname + ") " + nic.Name + " Connected"); err != nil {
					os.Stderr.WriteString("Could not write to SysLog: " + err.Error() + "\n")
				}
				w.Close()
			} else {
				os.Stderr.WriteString("Could not connect to SysLog: " + err.Error() + "\n")
			}
		}

		logFile := ""
		if nic.FileLog != nil && len(*nic.FileLog) > 0 {
			logFile = *nic.FileLog
		} else if nic.FileLog == nil && len(config.FileLog) > 0 {
			logFile = config.FileLog
		}
		if logFile != "" {
			file, err := os.OpenFile(logFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
			if err != nil {
				os.Stderr.WriteString("Error opening " + logFile + " for writing: " + err.Error() + "\n")
			}
			file.WriteString(now.Format("2006-01-02 15:04:05") + " - " + nicAddr + " (" + hostname + ") " + nic.Name + " Connected\n")
			file.Close()
		}

		if nic.Cmd != nil && currentlyExecuting[nicAddr] == false {
			currentlyExecuting[nicAddr] = true
			cmd := append([]string{}, nic.Cmd...)
			for i, v := range cmd {
				cmd[i] = strings.Replace(strings.Replace(v, "%hostname", hostname, -1), "%nic", nicAddr, -1)
			}
			go func(nicAddr string, cmd []string) { // Run in background
				if result, err := exec.Command(cmd[0], cmd[1:]...).Output(); err != nil {
					os.Stderr.WriteString("Error:" + err.Error())
				} else if len(result) > 0 {
					os.Stdout.Write(result)
				}
				delete(currentlyExecuting, nicAddr)
			}(nicAddr, cmd)
		}
	}
}
