// Packge cronmon is the core of the cronmon application, providing individual
// components that work independently, while communicating with eachother
// concurrently over channels.
//
// Mechanism of Operation
//
// Cronmon works similarly to your average service manager: it manages a group
// of programs to make sure they're alive; however, there are some differences
// that will be outlined below.
//
// Service Files
//
// In cronmon, a service file is an executable. Cronmon watches for executable
// files in the "scripts" directory, which is by default
// "$XDG_CONFIG_HOME/cronmon/scripts/". The directory is actively watched for
// changes, and service restarts will be performed accordingly.
//
// Note that when a regular editor writes to one of the scripts, it may perform
// multiple operations for atomicity, which may interfere and cause cronmon to
// restart the process multiple times rapidly. This is to be expected.
//
// Interruption
//
// When cronmon is suddenly (ungracefully) interrupted, its Pdeathsig mechanism
// will take down its managed processes as well, meaning that when cronmon is
// restored, there won't be the same processes running twice. Although this may
// not be a very ideal and portable solution, it is the simplest one.
//
// When a subprocess is disowned from the processes that cronmon has spawned,
// its parent process will be cronmon itself, not init (PID 1). This is
// accomplished using the non-portable SET_CHILD_SUBREAPER feature. This
// disowned process won't be killed when the process stops, but it will be
// killed when cronmon is interrupted.
//
// Journal Files
//
// During its operation, cronmon logs its actions into a journal file. Each
// cronmon process may take exclusive ownership of each journal file. The
// purpose of this journal file is to allow cronmon access to previous state
// while also providing a human and machine readable log format. The default
// path for the log file is "$XDG_CONFIG_HOME/cronmon/journal.json".
//
// Journal events are described in events.go, where each event is prefixed with
// "Event" and comes with a Type() method to aid writing these events down in
// any format. An implementation exists in package journal to read and write
// journals in the line-delimited JSON format.
package cronmon
