// Packge cronmon is the core of the cronmon application, providing individual
// components that work independently, while communicating with eachother
// concurrently over channels.
//
// Mechanism of Operation
//
// Journal Files
//
// In order to save the state of operation between cronmon restarts, cronmon
// logs its actions into a journal file. This journal file is read on every
// start to restore the previous state. Each cronmon process may take exclusive
// ownership on one journal file.
//
// Journal events are described in events.go, where each event is prefixed with
// "Event" and comes with a Type() method to aid writing these events down in
// any format. An implementation exists in package journal to read and write
// journals in the line-delimited JSON format.
//
// PID Files
//
// In order to ensure that cronmon knows whether or not a process is still alive
// even when it's dead, it relies on the Unix assumption that files are
// referenced counted.
//
// Take for example, that a file is created, opened, and then unlinked
// immediately without being closed. By creating and opening the file, the
// reference count will be 1, and unlinking it won't immediately delete the
// file. However, when closed, the reference count will drop to 0, and the file
// will be deleted.
//
// If the same file were to be given to a process, it will be kept open until
// the process explicitly closes it or exits. When this happens, the file will
// be gone, and cronmon will know that the process is dead if it can't find the
// file when it's back up. Vice versa, cronmon will know that the process is
// still alive if the file is still there, and it will take over the process to
// monitor it further.
//
// As for the implementation details, a "status directory" is made to contain
// "status files." This directory would be whatever os.TempDir() returns, joined
// with "cronmon" and the ID of the journaler. Inside it will contain empty
// files with the name of each children process being the filename.
//
// Multiple Monitor instances within the same process may share the same status
// directory.
//
// The status directory tree may look like this in some systems:
//
//    - /
//        - tmp/
//            - cronmon/
//                - file-WmcPPuLyMkNgOJM4HEacUQ/
//                    - process1
//                    - script2
//                    - thing3.sh
//
package cronmon
