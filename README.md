# noisemaker
A quick Go app for generating operational "noise" on a deployed machine, for testing threat detection.

## About the Project

Noisemaker is intended to serve as an easily buildable cross-platform tool for generating consistent signals for threat detection tools to be able to detect. It supports a small set of commands which generate nearly-consistent output (excluding OS-specific formatting, such as Windows vs POSIX usernames) across any operating system which can build the code.

### Decisions Made

- Chose Go programming language, because of ease of install and the ubiquity of Go packages like `os` and `encoding/csv`
- Built a struct representing the Activity Log and build serializers for loading/saving it to a file
- Used the `flags` package to set up the command-line options
- Chose to allow only one command per invocation to reduce complexity
- Chose CSV output because of platform-independence and ease of readability (either as text or in a spreadsheet program); could have done JSON, but didn't want to take that much time for mostly tabular data, would lose quick comparison ability
- Thought to allow user to specify whether to append or overwrite log file because I was tired of checking whether or not the header row was there! Tradeoff of using CSV meant that I had to do a couple of different approaches with the activity log before I was satisfied!
- Started with switch case and monolithic main() function for ease of getting all the parts correct; ran out of time to refactor and clean up. I'd love to work on this some more and get this separated out into extra files, but it should be readable as-is.
- Left various notes for future enhancements (ie. allowing HTTP headers for the send command, refactors, automated testing)

## Getting Started

To get started, follow these steps:

### Prerequisites

First, install the Go runtime on your host machine:

* Windows (from Admin Powershell)
    ```
    choco install golang
    ```
* Mac (from Admin Terminal)
    ```
    brew install go
    ```
* Linux (APT, DNF, YUM) (from shell)
    ```
    sudo apt-get install go
    sudo dnf install go
    sudo yum install go
    ```

### Installation

1. Clone the repo
    ```
    git clone https://github.com/nickiannone/noisemaker.git
    cd noisemaker
    ```

## Usage

To launch the app, run this command in your system's terminal, within the noisemaker repo directory.

```
    go run . [options] <command> [args...]
```

This version of Noisemaker currently supports five commands:

- execute (path-to-executable) [args...]                Spawns a process to execute the given command.
- create (path) [contents]                              Creates a file at the given path, with the given contents. Replaces if found.
- update (path) [contents]                              Updates an existing file at the given path, replacing its contents with the given contents.
- delete (path)                                         Deletes the file at the given path.
- send (method) (destaddr) [destport] [protocol] [body]     Sends an HTTP(S) network request.

The available options are as follows:

- -overwrite        Forces overwriting (instead of appending) of the specified activity log file.
- -logfile=(path)   Sets the activity log file path to use. Default is `./activity-log.csv`.

### Commands

1. execute (path) [args...]

Executes the given command specified by (path), optionally taking a variable list of arguments as space-delimited string tokens. Spawns an unmonitored child process, and records the PID of that process in the activity log.

2. create (path) [contents]

Creates a file at the given (path), optionally writing the contents specified in [contents]. Will fail if the path is missing or invalid, if the file is inaccessible by the current user, or the file already exists. Records result to the activity log.

3. update (path) [contents]

Replaces an existing file at the given (path), overwriting the contents if specified (and writing an empty file if not specified). Will fail if the path is missing or invalid, the file is inaccessible by the current user, or the file doesn't exist. Records result to the activity log.

4. delete (path)

Deletes an existing file at the given (path). Will fail if the path is missing or invalid, the file is inaccessible by the current user, or the file doesn't exist. Records result to the activity log.

5. send (method) (destaddr) [destport] [protocol] [body]

Sends a request using the given [protocol] (http or https, default: http) using the given HTTP method (default: GET), to the specified destination address and port (default: 80), and optionally (for POST/PUT) using [body] (default: "") as the body of the request. Echoes the response to the console, and records relevant information to the activity log.

### Activity Log

The activity log (by default, `./activity-log.csv`) stores the outcomes of all activities performed by the app, in CSV format:

```csv
timestamp,activity,os,username,processName,processCmd,pid,path,status,method,sourceAddr,sourcePort,destAddr,destPort,bytesSent,protocol
2024-11-05T16:20:14-06:00,execute,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build2954598208\b001\exe\main.exe,go version,39024,,,,,0,,0,0,
2024-11-05T16:20:26-06:00,create,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build3623895199\b001\exe\main.exe,create ./test.txt,1040,,created,,,0,,0,0,
2024-11-05T16:20:34-06:00,create,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build2855970878\b001\exe\main.exe,create ./README.md,37852,,exists,,,0,,0,0,
2024-11-05T16:20:40-06:00,create,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build2840590342\b001\exe\main.exe,create /root,42612,,error,,,0,,0,0,
2024-11-05T16:20:51-06:00,create,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build3173921831\b001\exe\main.exe,create ./test.txt Hello World!,25056,,exists,,,0,,0,0,
2024-11-05T16:21:04-06:00,update,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build1501726242\b001\exe\main.exe,update ./test.txt Hello World!,40988,,updated,,,0,,0,0,
2024-11-05T16:21:17-06:00,update,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build2127814719\b001\exe\main.exe,update ./nonexistent-file Missing?,44924,,not_found,,,0,,0,0,
2024-11-05T16:21:23-06:00,delete,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build2416591825\b001\exe\main.exe,delete ./test.txt,19480,,deleted,,,0,,0,0,
2024-11-05T16:21:29-06:00,delete,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build3920707031\b001\exe\main.exe,delete ./nonexistent-file,37896,,not_found,,,0,,0,0,
2024-11-05T16:21:35-06:00,delete,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build1932926980\b001\exe\main.exe,delete C:\Windows\system.ini,38752,,error,,,0,,0,0,
2024-11-05T16:22:06-06:00,send,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build1869099616\b001\exe\main.exe,send GET www.google.com,6924,http://www.google.com:80,sent,GET,[2600:1700:afd0:4ff0:a89a:cad2:44eb:b804],52671,www.google.com,80,0,http
2024-11-05T16:22:12-06:00,send,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build1410023602\b001\exe\main.exe,send GET www.google.com 80,43680,http://www.google.com:80,sent,GET,[2600:1700:afd0:4ff0:a89a:cad2:44eb:b804],52675,www.google.com,80,0,http
2024-11-05T16:22:18-06:00,send,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build1666584356\b001\exe\main.exe,send GET www.google.com 80 http,36088,http://www.google.com:80,sent,GET,[2600:1700:afd0:4ff0:a89a:cad2:44eb:b804],52676,www.google.com,80,0,http
2024-11-05T16:22:23-06:00,send,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build3557941028\b001\exe\main.exe,send POST www.postman-echo.com/post 443 https Hello World!,41356,https://www.postman-echo.com:443/post,sent,POST,192.168.1.67,52680,www.postman-echo.com/post,443,12,https
2024-11-05T16:22:29-06:00,send,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build2762430116\b001\exe\main.exe,send GET www.google.com 443 http,36804,http://www.google.com:443,error,GET,,0,www.google.com,443,0,http
2024-11-05T16:22:34-06:00,send,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build2530559101\b001\exe\main.exe,send GET INVALID_URL,5672,http://INVALID_URL:80,error,GET,,0,INVALID_URL,80,0,http
2024-11-05T16:22:39-06:00,send,windows,DESKTOP-FESHU4L\Nick,C:\Users\Nick\AppData\Local\Temp\go-build2680958679\b001\exe\main.exe,send GET www.google.com 65536,35940,http://www.google.com:65536,error,GET,,0,www.google.com,65536,0,http

```

When the application starts, it checks the activity log file (if it exists) for consistency, loads all activity log entries, and then executes the command specified with the given arguments. The overwrite flag will instead wipe the existing activity log file, and rewrite all records.

## Testing

To launch the tests, run this command in your system's terminal, within the noisemaker repo directory.

```
    go test ./...
```
