package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"os/user"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const HeaderStr = "timestamp,activity,os,username,processName,processCmd,pid,path,status,sourceAddr,sourcePort,destAddr,destPort,bytesSent,protocol,responseStatusCd,responseBody"

type ActivityLogEntry struct {
	timestamp   		string  `csv:"timestamp"`   		// RFC3339 timestamp
	activity    		string  `csv:"activity"`    		// [execute, create, modify, delete, send]
	os	        		string  `csv:"os"`          		// operating system name
	username    		string  `csv:"username"`    		// current username
	processName 		string  `csv:"processName"` 		// process name
	processCmd  		string  `csv:"processCmd"`  		// full process cmd string (with args)
	processId   		int     `csv:"pid"`         		// pid of created process
	// create, modify, delete only:
	path   				string  `csv:"path"`   				// path to the file (also used by "send" to include the full URL)
	status 				string  `csv:"status"` 				// [created, modified, deleted, sent, not_found, invalid_path, no_access, error]
	// send only:
	method	   			string  `csv:"method"`	 			// method (GET, POST, etc.)
	sourceAddr 			string  `csv:"sourceAddr"` 			// source IP address (resolved)
	sourcePort 			int     `csv:"sourcePort"` 			// source port
	destAddr   			string  `csv:"destAddr"`   			// destination IP address (resolved)
	destPort   			int     `csv:"destPort"`   			// destination port
	bytesSent  			int     `csv:"bytesSent"`  			// number of bytes transmitted
	protocol   			string  `csv:"protocol"`   			// the protocol used (http:, ftp:, udp:, etc.)
	responseStatusCd 	int     `csv:"responseStatusCd"`	// the response status code from the request
	responseBody		string	`csv:"responseBody"`		// the response body (with newlines and commas escaped)
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
	activityLogEntry.os = currentOS
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
		method := commandArgs[0]
		destAddr := commandArgs[1]
		destPort, err := strconv.Atoi(commandArgs[2])
		check(err)
		protocol := commandArgs[3]
		data := commandArgs[4]

		// Record the parsed identifying information
		activityLogEntry.method = method
		activityLogEntry.destAddr = destAddr
		activityLogEntry.destPort = destPort
		activityLogEntry.protocol = protocol

		// Log the details of what we're sending
		fmt.Printf("Sending %d bytes of data to %s %s (port %d) using protocol %s...\n", len(data), method, destAddr, destPort, protocol)

		// Send it!
		messageResponse, err := sendMessage(method, destAddr, destPort, protocol, nil, data)
		if err != nil {
			// TODO: Add more specific error handling?
			activityLogEntry.status = messageResponse.status
		} else {
			activityLogEntry.status = "sent"
		}

		// Record the stuff we resolved inside the call to sendMessage()
		activityLogEntry.path = messageResponse.path
		activityLogEntry.sourceAddr = messageResponse.sourceAddr
		activityLogEntry.sourcePort = messageResponse.sourcePort
		activityLogEntry.bytesSent = messageResponse.bytesSent
		activityLogEntry.responseStatusCd = messageResponse.responseStatusCode
		activityLogEntry.responseBody = escapeRawText(messageResponse.responseBody)
	case "help":
		// Print the help text?
		// TODO
	default:
		check(fmt.Errorf("invalid command specified: %s", command))
	}

	writeActivityLog(activityLogFile, activityLogEntry)
}

func escapeCommandString(cmd string, args []string) string {
	consolidated := cmd + " " + strings.Join(args, " ")
	return escapeRawText(consolidated)
}

// Escapes commas and newlines
func escapeRawText(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, ",", "\\,"), "\n", "\\n")
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

	fmt.Printf("%d bytes written to new file %s", bytesWritten, path)
	return "created", nil
}

func updateFile(path string, contents string) (string, error) {
	// TODO: How do we update the contents?
	//  - I'm just doing a full file overwrite with the new contents for simplicity!
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

	fmt.Printf("%d bytes written to updated file %s", bytesWritten, path)
	return "updated", nil
}

func deleteFile(path string) (string, error) {
	err := os.Remove(path)
	if err != nil {
		// TODO: Change this to spit out appropriate messages ("not_found", "invalid_path", "no_access", "error")
		return "error", err
	}

	fmt.Printf("File %s deleted", path)
	return "deleted", nil
}

type MessageResponse struct {
	responseBody		string
	sourceAddr			string
	sourcePort			int
	bytesSent			int
	status				string
	path				string
	responseStatusCode	int
}

func makeErrorResponse(status string, path string) *MessageResponse {
	response := new(MessageResponse)
	response.sourceAddr = ""
	response.sourcePort = 0
	response.bytesSent = 0
	response.status = status
	response.path = path
	response.responseBody = ""
	response.responseStatusCode = -1

	return response
}

func makeSuccessResponse(status string, responseBody string, sourceAddr string, sourcePort int, bytesSent int, path string, responseStatusCode int) *MessageResponse {
	response := new(MessageResponse)
	response.sourceAddr = sourceAddr
	response.sourcePort = sourcePort
	response.bytesSent = bytesSent
	response.status = status
	response.path = path
	response.responseBody = responseBody
	response.responseStatusCode = responseStatusCode

	return response
}

func sendMessage(method string, destAddr string, destPort int, protocol string, headers any, body string) (*MessageResponse, error) {
	// Add the port number into the destination address string
	destAddrWithPort, err := injectPortIntoAddress(destAddr, destPort, protocol)
	if err != nil {
		invalidPathStr := fmt.Sprintf("path %s port %d protocol %s", destAddr, destPort, protocol)
		return makeErrorResponse("invalid_address", invalidPathStr), err
	}
	path := protocol + "://" + destAddrWithPort

	// Determine how to actually emit the request
	switch protocol {
	case "http", "https":
		return sendHttpMessage(method, path, headers, body)
	default:
		// Return an error
		return makeErrorResponse("unknown_protocol", path), fmt.Errorf("unknown protocol: %s", protocol)
	}
}

func sendHttpMessage(method string, path string, headers any, body string) (*MessageResponse, error) {
	// Shove everything into an HTTP request
	reqBodyBuffer := bytes.NewBufferString(body)
	req, err := http.NewRequest(method, path, reqBodyBuffer)
	if err != nil {
		return makeErrorResponse("invalid_request", path), err
	}
	// TODO: Determine how we want the user to specify headers as CLI args!
	addHeadersAsNeeded(req, headers)

	// Set up the tracer, so we get the current machine's external connection info
	var sourceAddr string
	var sourcePort int = 0
	trace := &httptrace.ClientTrace {
		GetConn: func(hostPort string) {},
		GotConn: func(connInfo httptrace.GotConnInfo) {
			// Get the local address and port, as "<ip4>:<port>"
			localConnStr := connInfo.Conn.LocalAddr().String()
			connStrPieces := strings.Split(localConnStr, ":")
			sourceAddr = connStrPieces[0]
			sourcePort, err = strconv.Atoi(connStrPieces[1])
			if err != nil {
				// TODO: What do we do if the port isn't present?
				fmt.Printf("Local host is addr %s port unknown", sourceAddr)
			} else {
				fmt.Printf("Local host is addr %s port %d", sourceAddr, sourcePort)
			}
		},
		ConnectStart: func(network string, addr string) {},
		ConnectDone: func(network string, addr string, err error) {},
	}

	// Wrap the request with the tracer
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	// Emit the request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return makeErrorResponse("error", path), err
	}
	defer resp.Body.Close()

	// Read the response body
	var responseBodyStr string
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		responseBodyStr = ""
	} else {
		responseBodyStr = string(responseBody)
	}

	// Return a success
	return makeSuccessResponse("sent", responseBodyStr, sourceAddr, sourcePort, int(req.ContentLength), path, resp.StatusCode), nil
}

// TODO: Incorporate headers before sending a request?
func addHeadersAsNeeded(req *http.Request, headers any) {
	// panic("unimplemented")
}

func injectPortIntoAddress(addr string, port int, protocol string) (string, error) {
	switch protocol {
	case "http", "https":
		u, err := url.Parse(addr)
		if err != nil {
			return "", fmt.Errorf("unable to parse address %s", addr)
		}

		return strings.Replace(addr, u.Host, u.Host+":"+strconv.Itoa(port), 1), nil
	default:
		return "", fmt.Errorf("unknown protocol: %s", protocol)
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
	// Create a writer and attach it to the log file
	writer := csv.NewWriter(logFile)
	defer writer.Flush()

	// Convert the log entry to a row of CSV-formatted strings
	row := []string{}
	value := reflect.ValueOf(logInfo)
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		row = append(row, fmt.Sprintf("%v", field.Interface()))
	}

	// Write it to the activity log
	err := writer.Write(row)
	if err != nil {
		check(fmt.Errorf("unable to write csv to file"))
	}
}

// https://stackoverflow.com/a/31196754
func startProcess(cmd string, args []string) (*os.Process, error) {
	procAttr := new(os.ProcAttr)
	procAttr.Files = []*os.File{os.Stdin, os.Stdout, os.Stderr}
	return os.StartProcess(cmd, args, procAttr)
}
