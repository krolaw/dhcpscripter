# DHCP Scripter (and Logger)

## History

At 2013-01-16T09:28:47 [Stewart Knapman](http://www.stewartknapman.com/) [skyped](http://www.skype.com/) me [The coolest thing ever](http://blog.xk72.com/post/13313246225/the-coolest-thing-ever).  Essentially, Karl von Randow had automated a customised entrance sound to be played for each colleague as they entered the office.

What would seem like magic to outsiders, was cleverly orchestrated, using a feature in the DHCP server to execute different commands based on the hardware address (nic) of the device connecting to the network; which was the entrant's smart phone.  It was a trick I wanted badly...

Trouble was, both home and work dhcp services were embedded on ADSL routers, so I wasn't going to be able to do it without setting up a dhcp service on a server and disabling the one on the router.  It occurred to me that not everyone would have the expertise (or the security clearance in some places), to implement what Karl did.

## Introduction

DHCP Scripter (DS) brings music intro themes to the masses.  It doesn't take the place of your dhcp server.  DS just listens for the DHCP request of the newly entered device/phone, extracts the nic, and executes the command from its config file.  Because, DHCP requests are broadcast to every computer on the network, DS can be run on any machine, or multiple machines :-).  You don't need to touch the server :-)

You will however need administrator rights to the machine you are running DS on, as it needs to listen on port 67.  (Ports lower than 1024 usually require administrator privileges.)

## Caveats

Your DHCP server should have a longer lease time than you expect anyone to stay.  For example, a DHCP server with a lease period of 1 hour, requires devices to check in at least once per hour to remain connected.  Therefore, anyone staying longer than an hour, will cause DS to have a false positive.  A lease time of one day is recommended.

Apple iOS devices' DHCP handling is broken in several versions and can cause DS to fire every few minutes.  It's not so bad with later versions of iOS.  Princeton University has produced an [interesting document](http://www.net.princeton.edu/apple-ios/ios40-requests-DHCP-too-often.html) detailing Apple's inability to solve the problem correctly.  Other devices such as Android appear to work flawlessly.

## Configuration

DHCP Scripter runs from the command line and only has one flag: _-conf_  
This controls which configuration file to use, which by default is: dhcps.conf

The configuration file uses [JSON](http://www.json.org) format.  Parameters that DS does not recognise are ignored, which can be handy if you want to add comments, using say a "comment" tag.  All DS parameters start with a capital letter.

### Top Level Parameters

* "Port":
	Default 67.  This allows DS to run as an unprivileged user.  However, it only works if the DHCP packets are re-routed to your chosen Port:
	* Linux: sudo iptables -I PREROUTING -t nat -p udp -s 0.0.0.0 --sport 68 -d 0.0.0.0 --dport 67 -j DNAT --to 0.0.0.0:6767
	* Mac (untested): sudo ipfw add 100 fwd 0.0.0.0,6767 udp from 0.0.0.0 to 0.0.0.0 67 in

* "SysLog":
	Default false.  Logs DHCP requests to System Logger.

* "FileLog":
	Defines file to log DHCP requests to.  If empty does nothing.

* "NICs":
	A map using nics as keys with NIC Objects as the values.  A key of "default" is used when no listed nic matches the connecting device.
	
### NIC Object Parameters

* "SysLog":
	Default inherit.  Overrides Top Level SysLog behaviour for this nic.
	
* "FileLog":
	Default inherit.  Overrides Top Level FileLog behaviour for this nic. NULL is permitted.

* "Name":
	Optional Name, recorded in logs.

* "Cmd":
	Optional command to run when device connects.  Note that the format of the command is an array and that each parameter of the command is a separate value.  If _%nic_ or _%hostname_ is included, they will be replaced by their respective values from the DHCP packet.

### Example Config

	{
		"Port":6767,
		"SysLog":true,
		"NICs" : {
			"20:64:33:72:34:f1" : {
				"Name":"Richard Mobile",
				"Cmd":["espeak","-s100","-p2000","-ven-uk","Hurray! Dadd-ee Iz home."]
			},
			"5c:e2:f5:55:93:5b" : {
				"Name":"Suzanne Mobile",
				"Cmd":["espeak","-s100","-p2000","-ven-uk","Hurray! Mum-ee Iz home."]
			},
			"default" : {
				"FileLog":"UnknownDevices.txt"
			}
		}
	}
