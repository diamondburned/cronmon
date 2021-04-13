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

On start, cronmon will attempt to detect PID files that have been preemptively unlinked before they are passed to the application. According to [this StackOverflow][delete file on close] article, a file is reference-counted, so while the file is opened, it can be `unlink`ed, which won't delete it right away until the file descriptor is closed. We can hand this file descriptor off to the program, so when the program dies, the file will be deleted, signaling that it is dead.

This behavior probably breaks on non-Unixes like Windows.

[delete file on close]: https://stackoverflow.com/questions/3181641/how-can-i-delete-a-file-upon-its-close-in-c-on-linux

## Cron File

```sh
0 0 0 0 0 cronmon
0 0 0 0 0 (sleep 30; cronmon) # more accuracy if needed
```
