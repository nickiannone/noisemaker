package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const HeaderStr = "timestamp,activity,os,username,processName,processCmd,pid,path,status,sourceAddr,sourcePort,destAddr,destPort,bytesSent,protocol,"

type ActivityLogEntry struct {
	timestamp   string `csv:"timestamp"`   // RFC3339 timestamp
	activity    string `csv:"activity"`    // [execute, create, modify, delete, send]
	osId        string `csv:"os"`          // operating system name
	username    string `csv:"username"`    // current username
	processName string `csv:"processName"` // process name
	processCmd  string `csv:"processCmd"`  // full process cmd string (with args)
	processId   int    `csv:"pid"`         // pid of created process
	// create, modify, delete only:
	path   string `csv:"path"`   // path to the file
	status string `csv:"status"` // [created, modified, deleted, not_found, invalid_path, no_access, error]
	// send only:
	sourceAddr string `csv:"sourceAddr"` // source IP address or URL (how do we find this?)
	sourcePort int    `csv:"sourcePort"` // source port
	destAddr   string `csv:"destAddr"`   // destination IP address or URL
	destPort   int    `csv:"destPort"`   // destination port
	bytesSent  int    `csv:"bytesSent"`  // number of bytes transmitted
	protocol   string `csv:"protocol"`   // the protocol used (http:, ftp:, udp:, etc.)
}

// Usage: noisemaker [opts...] <command> [args...]
// Options:
//   - -logfile=<path>	(sets activity log path; default './activity-log.csv')
//   - -overwrite		(sets activity log to overwrite log file if existing, instead of appending; default false)
//
// Commands:
//   - execute (runs command-line string)
//   - create (creates file)
//   - modify (modifies file)
//   - delete (deletes file)
//   - send (invokes curl)
func main() {
	// Determine which OS we're on ('darwin', 'linux', etc.)
	currentOS := runtime.GOOS

	// Determines the current user
	currentUser, err := user.Current()
	check(err)

	// Parse CLI flags
	logFilePathPtr := flag.String("logfile", "./activity-log.csv", "the path to the activity log CSV file")
	overwritePtr := flag.Bool("overwrite", false, "whether to overwrite (true) or append to (false) the activity log CSV file (default false)")

	// Parse flags
	flag.Parse()
	logFilePath := *logFilePathPtr
	overwrite := *overwritePtr
	logFileMode := os.O_APPEND | os.O_CREATE | os.O_WRONLY
	if overwrite {
		logFileMode = os.O_RDWR | os.O_CREATE
	}

	// Get the command and args
	remainingArgs := flag.Args()
	command := remainingArgs[0]
	commandArgs := remainingArgs[1:]

	// Check the activity log to see if it already has a header; if not, write one.
	writeHeader := true
	if !overwrite {
		peekActivityLogFile, err := os.Open(logFilePath)
		check(err)
		defer peekActivityLogFile.Close()

		scanner := bufio.NewScanner(peekActivityLogFile)
		if scanner.Scan() {
			firstLine := scanner.Text()
			if firstLine == "" {
				fmt.Println("Empty first line, assuming no header")
				writeHeader = true
			} else if isHeader(firstLine) {
				fmt.Println("Header detected on first line")
				writeHeader = false
			} else {
				fmt.Println("Header not detected on first line, adding header line at current end of file")
				// TODO: Shuffle the file's existing contents downwards and add a header?
				writeHeader = true
			}
		} else if scanner.Err() != nil {
			fmt.Println("Unable to open existing file for appending, it does not exist!")
			writeHeader = true
		}
	}

	// Open the activity log for writing
	activityLogFile, err := os.OpenFile(logFilePath, logFileMode, 0644)
	check(err)
	defer activityLogFile.Close()

	// If we're writing a header, do it now!
	if writeHeader {
		_, err := activityLogFile.WriteString(HeaderStr + "\n")
		check(err)
	}

	// Get the current process name and PID
	currentProcessId := os.Getpid()
	currentProcessName, err := os.Executable()
	check(err)

	// Create the initial activity log entry
	activityLogEntry := new(ActivityLogEntry)
	activityLogEntry.timestamp = time.Now().Format(time.RFC3339)
	activityLogEntry.activity = command
	activityLogEntry.username = currentUser.Username
	activityLogEntry.osId = currentOS
	activityLogEntry.processName = currentProcessName
	activityLogEntry.processCmd = escapeCommandString(command, commandArgs)
	activityLogEntry.processId = currentProcessId

	// Determine what process to run
	switch command {
	case "execute":
		// Call startProcess and capture the output
		procCmd := commandArgs[0]
		procArgs := commandArgs[1:]
		activityLogEntry.processCmd = escapeCommandString(procCmd, procArgs)

		process, err := startProcess(procCmd, procArgs)
		check(err)

		activityLogEntry.processId = process.Pid
	case "create":
		// Call createFile and capture the output
		path := commandArgs[0]
		contents := commandArgs[1]

		status, err := createFile(path, contents)
		if err != nil {
			// TODO: Add more specific create error info to log entry!
			activityLogEntry.status = status // [not_found, invalid_path, no_access, error]
		} else {
			activityLogEntry.status = "created"
		}
	case "update":
		// Call updateFile and capture the output
		path := commandArgs[0]
		contents := commandArgs[1]

		status, err := updateFile(path, contents)
		if err != nil {
			activityLogEntry.status = status // [not_found, invalid_path, no_access, error]
		} else {
			activityLogEntry.status = "updated"
		}
	case "delete":
		// Call deleteFile and capture the output
		path := commandArgs[0]
		status, err := deleteFile(path)
		if err != nil {
			// TODO: Add more specific delete error info to log entry!
			activityLogEntry.status = status // [not_found, invalid_path, no_access, error]
		} else {
			activityLogEntry.status = "deleted"
		}
	case "send":
		// Send a message to a given receiver
		destAddr := commandArgs[0]
		destPort, err := strconv.Atoi(commandArgs[1])
		check(err)
		protocol := commandArgs[2]
		data := commandArgs[3]

		// Record the parsed identifying information
		activityLogEntry.destAddr = destAddr
		activityLogEntry.destPort = destPort
		activityLogEntry.protocol = protocol

		sourceAddr, sourcePort, bytesSent, status, err := sendMessage(destAddr, destPort, protocol, data)
		if err != nil {
			// TODO: Add more specific error handling?
			activityLogEntry.status = status
		} else {
			activityLogEntry.status = "sent"
		}

		// Record the stuff we resolved inside the call to sendMessage()
		activityLogEntry.sourceAddr = sourceAddr
		activityLogEntry.sourcePort = sourcePort
		activityLogEntry.bytesSent = bytesSent
	case "help":
		// Print the help text?
		// TODO
	default:
		fmt.Printf("No command specified!")
		return
	}

	writeActivityLog(activityLogFile, activityLogEntry)
}

func escapeCommandString(cmd string, args []string) string {
	consolidated := cmd + " " + strings.Join(args, " ")
	escapedCommas := strings.ReplaceAll(consolidated, ",", "\\,")
	escapedNewlines := strings.ReplaceAll(escapedCommas, "\n", "\\n")

	return escapedNewlines
}

func createFile(path string, contents string) (string, error) {
	f, err := os.Create(path)
	if err != nil {
		// TODO: Change this to spit out appropriate messages ("not_found", "invalid_path", "no_access", "error")
		return "error", err
	}
	defer f.Close()

	bytesWritten, err := f.WriteString(contents)
	if err != nil {
		return "error", err
	}

	fmt.Println("%d bytes written to new file %s", bytesWritten, path)
	return "created", nil
}

func updateFile(path string, contents string) (string, error) {
	// TODO: How do we update the contents?
	f, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		// TODO: Change this to spit out appropriate messages ("not_found", "invalid_path", "no_access", "error")
		return "error", err
	}
	defer f.Close()

	bytesWritten, err := f.WriteString(contents)
	if err != nil {
		return "error", err
	}

	fmt.Println("%d bytes written to updated file %s", bytesWritten, path)
	return "updated", nil
}

func deleteFile(path string) (string, error) {
	err := os.Remove(path)
	if err != nil {
		// TODO: Change this to spit out appropriate messages ("not_found", "invalid_path", "no_access", "error")
		return "error", err
	}

	fmt.Println("File %s deleted", path)
	return "deleted", nil
}

// TODO: Clean up this signature by refactoring into a cleaner pattern? (responseBody, sourceAddr, sourcePort, bytesSent, status, responseStatusCode, err)
// TODO: Add Method as first argument everywhere!
func sendMessage(method string, destAddr string, destPort int, protocol string, body string) (string, string, int, int, string, int, error) {
	// Add the port number into the destination address string
	destAddrWithPort, err := injectPortIntoAddress(destAddr, destPort, protocol)
	if err != nil {
		return "", "", 0, 0, "invalid_input", 0, err
	}
	url := protocol + "://" + destAddrWithPort

	// Determine how to actually emit the request
	// TODO: Refactor!
	switch protocol {
	case "http", "https":
		// Shove everything into an HTTP request
		reqBodyBuffer := bytes.NewBufferString(body)
		req, err := http.NewRequest(method, url, reqBodyBuffer)
		if err != nil {
			return "", "", 0, 0, "invalid_request", -1, err
		}
		// TODO: Determine how we add headers!
		addHeadersAsNeeded(req)

		// Emit the request
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", "", 0, 0, "error", -1, err
		}
		defer resp.Body.Close()

		sourceAddr, sourcePort := extractSourceAddrAndPort(resp)
		responseStatusCode := resp.StatusCode
		responseBody, err := io.ReadAll(resp.Body)
		if err != nil {
			// TODO: Do we have access to the source hostname/port and number of bytes transferred here?
			return "", "", 0, 0, "error", responseStatusCode, err
		}

		// Return the generated response
		return string(responseBody), sourceAddr, sourcePort, int(req.ContentLength), "sent", responseStatusCode, nil
	default:
		// Return an error
		return "", "", 0, 0, "unknown_protocol", -1, fmt.Errorf("Unknown protocol: %s", protocol)
	}
}

func injectPortIntoAddress(addr string, port int, protocol string) (string, error) {
	switch protocol {
	case "http", "https":
		u, err := url.Parse(addr)
		if err != nil {
			return "", fmt.Errorf("Unable to parse address %s", addr)
		}

		return strings.Replace(addr, u.Host, u.Host+":"+strconv.Itoa(port), 1), nil
	default:
		return "", fmt.Errorf("Unknown protocol: %s", protocol)
	}
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// TODO: Make this less brittle somehow?
func isHeader(line string) bool {
	return line == HeaderStr
}

// Writes the activity log entry to the file.
func writeActivityLog(logFile *os.File, logInfo *ActivityLogEntry) {
	// Serialize to CSV
	csv, err := serializeActivityLogEntryToCSV(logInfo)
	check(err)

	// Write out to the file, buffered, and flush the buffer once (TODO: Verify flushing?)
	_, err = logFile.WriteString(csv + "\n")
	check(err)
}

// Serializes activity log entry to CSV
func serializeActivityLogEntryToCSV(logInfo *ActivityLogEntry) (string, error) {
	// TODO: Implement!
	return "", fmt.Errorf("unimplemented")
}

// https://stackoverflow.com/a/31196754
func startProcess(cmd string, args []string) (*os.Process, error) {
	procAttr := new(os.ProcAttr)
	procAttr.Files = []*os.File{os.Stdin, os.Stdout, os.Stderr}
	return os.StartProcess(cmd, args, procAttr)
}
