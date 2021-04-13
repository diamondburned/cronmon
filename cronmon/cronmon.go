// Packge cronmon is the core of the cronmon application, providing individual
// components that work independently, while communicating with eachother
// concurrently over channels.
//
// Mechanism of Operation
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
// with "cronmon" and its process ID. Inside it will contain empty files with
// the name of each children process being the filename.
//
// The status directory tree may look like this in some systems:
//
//    - /
//        - tmp/
//            - cronmon/
//                - 20354/
//                    - process1
//                    - script2
//                    - thing3.sh
//
package cronmon
