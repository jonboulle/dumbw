dumbw
=====

A super simple and dumb bandwidth monitor to integrate with the wonderful i3.

Tiny daemon which calculates an impossibly simplistic network usage rate for
interfaces on the system.

- Run a daemon by invoking `dumbw` the first time (daemons are tied to a user)
- Run a client with `dumbw` when a daemon is already running
- Invoke as `i3status-dumbw` to chain with `i3status` - e.g.:

```
$ egrep -A4 ^bar ~/.i3/config 
bar {
	status_command i3status | i3status-dumbw eth0
	position top
	tray_output primary
}
```

TODO:
 - make i3status wrapper more robust in the case of client failure
