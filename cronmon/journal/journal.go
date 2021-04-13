// Package journal provides an implementation of cromon's Journaler interface to
// write to a file. It also provides a file locking abstraction so that only one
// cronmon instance can run with the same journal file.
package journal
