# cronmon Draft

## Differences vs. Other Monitors

- Daemonless, stateful PID tracking
- Crontab-based

## Default Configuration Location

```
$ cat ~/.cronmon/sysmetd
#!/bin/sh
exec sysmet -flag "$shell_env" -listen unix:///tmp/a.sock
```

## Operation Mechanism

On start, cronmon attempts to acquire a local user file lock. Failure to acquire
the lock indicates that cronmon is still running, and the cronjob instance will
bail.

If cronmon starts successfully, it will execute a list of exec.Cmds parsed from
the configuration file. Each Cmd will have a unique name that is the filename
inside the cronmon directory. Each goroutine is in charge of monitoring its own
process.

The cronmon directory is fsnotified for changes. If a new daemon file is
created, then it will be immediately added into the watch list. If a file is
deleted, then it will be removed from the watch list as well. Along the children
commands, the main goroutine will be in charge of adding and removing these
goroutines from its internal state.

The acquired lock gives a cronmon instance exclusive writes over a journal. Any
other instance must only read from the journal. A corrupted journal (e.g. decode
fail) will be wiped and restarted, and the wipe operation is recorded into the
file.

The journal effectively contains a list of steps (or actions) that cronmon has
performed. This is critical to allow cronmon to restore itself: on the event of
a failure, cronmon will try to take over the PIDs recorded in the transaction
for all known files and pick up the job.

On start, cronmon will attempt to scan backwards the journal to find the status
for the processes that it would start.
	- If the process is not found or is found to have been stopped, then it will
	  be started.
	- If the only entry indicates that the process is already running with no
	  indication that it has stopped, then the process is looked up its parent
	  PID to be compared with the one in the journal.
	  	- If the PPID matches, then cronmon will try to terminate it.
		- If the PPID does not match, then the process is considered dead.

## Cron File

```sh
0 0 0 0 0 cronmon
0 0 0 0 0 (sleep 30; cronmon) # more accuracy if needed
```
