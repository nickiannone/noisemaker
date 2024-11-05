# noisemaker
A quick Go app for generating operational "noise" on a deployed machine, for testing threat detection.

## About the Project

Noisemaker is intended to serve as an easily buildable cross-platform tool for generating consistent signals for threat detection tools to be able to detect. It supports a small set of commands which generate nearly-consistent output across any operating system which can build the code.

This version of Noisemaker currently supports five commands:

- execute <command> [args...]                   Spawns a process to execute the given command.
- create <path> [contents]                      Creates a file at the given path, with the given contents. Replaces if found.
- update <path> [contents]                      Updates an existing file at the given path, replacing its contents with the given contents.
- delete <path>                                 Deletes the file at the given path.
- send <method> <addr> [port] [protocol] [body] Sends an HTTP(S) network request.

## Getting Started

