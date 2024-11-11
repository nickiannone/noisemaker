package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const HeaderStr = "timestamp,activity,os,username,processName,processCmd,pid,path,status,method,sourceAddr,sourcePort,destAddr,destPort,bytesSent,protocol"

type ActivityLogEntry struct {
	timestamp   		string  `csv:"timestamp"`   		// RFC3339 timestamp
	activity    		string  `csv:"activity"`    		// [execute, create, modify, delete, send]
	os	        		string  `csv:"os"`          		// operating system name
	username    		string  `csv:"username"`    		// current username
	processName 		string  `csv:"processName"` 		// process name
	processCmd  		string  `csv:"processCmd"`  		// full process cmd string (with args)
	processId   		int     `csv:"pid"`         		// pid of created process
	// create, modify, delete, send only:
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
	// responseStatusCd 	int     `csv:"responseStatusCd"`	// the response status code from the request
	// responseBody		string	`csv:"responseBody"`		// the response body (with newlines and commas escaped)
}

// Response data from send action
type MessageResponse struct {
	sourceAddr			string
	sourcePort			int
	bytesSent			int
	status				string
	path				string
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
//   - send (sends an HTTP(S) request)
func main() {
	// Determine which OS we're on ('darwin', 'linux', etc.)
	currentOS := runtime.GOOS

	// Get the current process name and PID
	currentProcessId := os.Getpid()
	currentProcessName, err := os.Executable()
	check(err)

	// Determines the current user
	currentUser, err := user.Current()
	check(err)

	// Parse log file flags
	logFilePathPtr := flag.String("logfile", "./activity-log.csv", "the path to the activity log CSV file")
	overwritePtr := flag.Bool("overwrite", false, "whether to overwrite (true) or append to (false) the activity log CSV file (default false)")
	flag.Parse()
	logFilePath := *logFilePathPtr
	overwrite := *overwritePtr

	// Get the command and args
	remainingArgs := flag.Args()
	if len(remainingArgs) < 1 {
		fmt.Printf("No command specified! Exiting...\n")
		return
	}
	command := remainingArgs[0]
	commandArgs := []string{}
	if len(remainingArgs) > 1 {
		commandArgs = remainingArgs[1:]
	}

	// Parse log entries from the existing log file, if any.
	existingLogEntries := []*ActivityLogEntry{}
	activityLogFileExists := fileExists(logFilePath)
	peekActivityLogFile, err := os.OpenFile(logFilePath, os.O_RDONLY, 0644)
	if activityLogFileExists && err != nil {
		scanner := bufio.NewScanner(peekActivityLogFile)
		if scanner.Scan() {
			firstLine := scanner.Text()
			fmt.Printf("First line: %s\n", firstLine)
			if !isCSVHeaderStr(firstLine) {
				// Try to parse it as a record, but fail gracefully
				row, err := splitCSVRow(firstLine)
				if err != nil {
					fmt.Printf("Unable to tokenize first row, syntax error in '%s'!\n", firstLine)
				}
				parsedLogEntry, err := deserializeFromCSV(row)
				if err != nil {
					fmt.Printf("Unable to deserialize first row, parser error in %v\n", row)
				}
				if parsedLogEntry != nil {
					fmt.Printf("Deserialized first row to %v\n", parsedLogEntry)
					existingLogEntries = append(existingLogEntries, parsedLogEntry)
				}
			}

			// Read the other rows
			for scanner.Scan() {
				existingRow := scanner.Text()
				// Try to parse it as a record, and skip ahead if we fail anywhere
				row, err := splitCSVRow(existingRow)
				if err != nil {
					fmt.Printf("Unable to tokenize row, syntax error in '%s'!\n", existingRow)
					continue
				}
				parsedLogEntry, err := deserializeFromCSV(row)
				if err != nil {
					fmt.Printf("Unable to deserialize first row, parser error in %v\n", row)
					continue
				}
				if parsedLogEntry != nil {
					fmt.Printf("Deserialized first row to %v\n", parsedLogEntry)
					existingLogEntries = append(existingLogEntries, parsedLogEntry)
				}
			}
		} else if scanner.Err() != nil {
			fmt.Println("Unable to open existing file for appending, it does not exist!")
		}
	}
	peekActivityLogFile.Close()

	// Open the activity log for writing
	var activityLogFile *os.File
	var writeHistoricalRecords bool
	if activityLogFileExists && !overwrite {
		fmt.Printf("Opening existing log file %s for appending...\n", logFilePath)
		activityLogFile, err = os.OpenFile(logFilePath, os.O_APPEND | os.O_CREATE | os.O_WRONLY, 0644)
		writeHistoricalRecords = false
	} else if activityLogFileExists && overwrite {
		fmt.Printf("Opening existing log file %s for overwriting...\n", logFilePath)
		activityLogFile, err = os.OpenFile(logFilePath, os.O_RDWR | os.O_CREATE, 0644)
		writeHistoricalRecords = true
	} else {
		fmt.Printf("Creating new log file %s...\n", logFilePath)
		activityLogFile, err = os.Create(logFilePath)
		writeHistoricalRecords = true
	}
	check(err)
	defer activityLogFile.Close()

	// Write the header and old records
	if writeHistoricalRecords {
		// Write header
		_, err = activityLogFile.WriteString(HeaderStr + "\n")
		check(err)

		// Write all other existing log entries
		for _, logEntry := range existingLogEntries {
			writeLogEntry(activityLogFile, logEntry)
		}
	}

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

		fmt.Printf("Running command %s with args %v\n", procCmd, procArgs)
		process, cancelFunc, processState, err := startProcess(procCmd, procArgs)
		check(err)

		// Close the connection, if we need to
		if cancelFunc != nil {
			// TODO: Verify this does what we think it does!
			// `defer cancelFunc` vs `defer cancelFunc()`!
			defer cancelFunc()
		}

		// Record the process info
		if processState != nil {
			activityLogEntry.processId = processState.Pid()
			activityLogEntry.status = processState.String()
		} else {
			activityLogEntry.processId = process.Pid
			activityLogEntry.status = "unable_to_run"
		}

	case "create":
		// Call createFile and capture the output
		if len(commandArgs) < 1 {
			check(fmt.Errorf("not enough arguments for create! Args: %v", commandArgs))
		}
		path := commandArgs[0]
		var contents string = ""
		if len(commandArgs) > 1 {
			contents = commandArgs[1]
		}

		status, err := createFile(path, contents)
		if err != nil {
			// TODO: Add more specific create error info to log entry!
			activityLogEntry.status = status // [not_found, invalid_path, no_access, error]
		} else {
			activityLogEntry.status = "created"
		}
	case "update":
		// Call updateFile and capture the output
		if len(commandArgs) < 2 {
			check(fmt.Errorf("not enough arguments for update! Args: %v", commandArgs))
		}
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
		if len(commandArgs) < 1 {
			check(fmt.Errorf("not enough arguments for delete! Args: %v", commandArgs))
		}
		path := commandArgs[0]
		status, err := deleteFile(path)
		if err != nil {
			// TODO: Add more specific delete error info to log entry!
			activityLogEntry.status = status // [not_found, invalid_path, no_access, error]
		} else {
			activityLogEntry.status = "deleted"
		}
	case "send":
		if len(commandArgs) < 2 {
			check(fmt.Errorf("not enough arguments for send! Args: %v", commandArgs))
		}

		// Get the arguments
		method := http.MethodGet
		if len(commandArgs) > 0 {
			method = commandArgs[0]
		}
		destAddr := "192.168.0.1"
		if len(commandArgs) > 1 {
			destAddr = commandArgs[1]
		}
		destPort := 80
		if len(commandArgs) > 2 {
			destPort, err = strconv.Atoi(commandArgs[2])
			check(err)
		}
		protocol := "http"
		if len(commandArgs) > 3 {
			protocol = commandArgs[3]
		}
		data := ""
		if len(commandArgs) > 4 {
			data = commandArgs[4]
		}

		// Record the parsed identifying information
		activityLogEntry.method = method
		activityLogEntry.destAddr = destAddr
		activityLogEntry.destPort = destPort
		activityLogEntry.protocol = protocol

		// Log the details of what we're sending
		fmt.Printf("Sending %d bytes of data to %s %s (port %d) using protocol %s...\n", len(data), method, destAddr, destPort, protocol)

		// Send it!
		// TODO: Add header encoding somehow!
		messageResponse, err := sendMessage(method, destAddr, destPort, protocol, nil, data)
		if err != nil {
			// TODO: Add more specific error handling?
			activityLogEntry.status = messageResponse.status
		} else {
			activityLogEntry.status = "sent"
		}

		// Record the resolved path details and how many bytes were sent
		activityLogEntry.path = messageResponse.path
		activityLogEntry.sourceAddr = messageResponse.sourceAddr
		activityLogEntry.sourcePort = messageResponse.sourcePort
		activityLogEntry.bytesSent = messageResponse.bytesSent
	case "help":
		// TODO: Print the help text?
	default:
		check(fmt.Errorf("invalid command specified: %s", command))
	}

	writeLogEntry(activityLogFile, activityLogEntry)
}

// =====================================================================
// Actions
// =====================================================================

// Create a file with given contents
func createFile(path string, contents string) (string, error) {
	if fileExists(path) {
		fmt.Printf("File %s already exists, unable to write!\n", path)
		return "exists", fmt.Errorf("file_already_exists: %s", path)
	}
	f, err := os.Create(path)
	if err != nil {
		// TODO: Change this to spit out appropriate messages ("not_found", "invalid_path", "no_access", "error")
		fmt.Printf("Error: %v\n", err)
		return "error", err
	}
	defer f.Close()

	bytesWritten, err := f.WriteString(contents)
	if err != nil {
		return "error", err
	}

	fmt.Printf("%d bytes written to new file %s\n", bytesWritten, path)
	return "created", nil
}

// Update a file with new contents, if it exists
func updateFile(path string, contents string) (string, error) {
	if !fileExists(path) {
		fmt.Printf("File %s not found for updating!\n", path)
		return "not_found", fmt.Errorf("file_not_found: %s", path)
	}
	
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

	fmt.Printf("%d bytes written to updated file %s\n", bytesWritten, path)
	return "updated", nil
}

// Delete a file, if it exists
func deleteFile(path string) (string, error) {
	if !fileExists(path) {
		fmt.Printf("File %s not found for deleting!\n", path)
		return "not_found", fmt.Errorf("file_not_found: %s", path)
	}

	err := os.Remove(path)
	if err != nil {
		// TODO: Change this to spit out appropriate messages ("not_found", "invalid_path", "no_access", "error")
		return "error", err
	}

	fmt.Printf("File %s deleted\n", path)
	return "deleted", nil
}

// Send an HTTP/HTTPS message to the given recipient
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

// ==================================================================================
// Helper methods
// ==================================================================================

// Helper for an error response from send
func makeErrorResponse(status string, path string) *MessageResponse {
	response := new(MessageResponse)
	response.sourceAddr = ""
	response.sourcePort = 0
	response.bytesSent = 0
	response.status = status
	response.path = path

	return response
}

// Helper for a success response from send
func makeSuccessResponse(status string, sourceAddr string, sourcePort int, bytesSent int, path string) *MessageResponse {
	response := new(MessageResponse)
	response.sourceAddr = sourceAddr
	response.sourcePort = sourcePort
	response.bytesSent = bytesSent
	response.status = status
	response.path = path

	return response
}

// TODO: Replace this with something that uses the field annotations!
func serializeToCSV(logInfo *ActivityLogEntry) []string {
	return []string{
		logInfo.timestamp,
		logInfo.activity,
		logInfo.os,
		logInfo.username,
		logInfo.processName,
		logInfo.processCmd,
		strconv.Itoa(logInfo.processId),
		logInfo.path,
		logInfo.status,
		logInfo.method,
		logInfo.sourceAddr,
		strconv.Itoa(logInfo.sourcePort),
		logInfo.destAddr,
		strconv.Itoa(logInfo.destPort),
		strconv.Itoa(logInfo.bytesSent),
		logInfo.protocol,
		// strconv.Itoa(logInfo.responseStatusCd),
		// logInfo.responseBody,
	}
}

// TODO: Refactor this to use some sort of mapping!
func deserializeFromCSV(row []string) (*ActivityLogEntry, error) {
	if len(row) < 16 {
		check(fmt.Errorf("not enough fields in row %v to load activity log entry! (16 required, %d found)", row, len(row)))
	}

	pidVal, err := strconv.Atoi(row[6])
	if err != nil {
		pidVal = 0
	}
	sourcePortVal, err := strconv.Atoi(row[11])
	if err != nil {
		sourcePortVal = 0
	}
	destPortVal, err := strconv.Atoi(row[13])
	if err != nil {
		destPortVal = 0
	}
	bytesSentVal, err := strconv.Atoi(row[14])
	if err != nil {
		bytesSentVal = 0
	}

	logInfo := new(ActivityLogEntry)
	logInfo.timestamp = row[0]
	logInfo.activity = row[1]
	logInfo.os = row[2]
	logInfo.username = row[3]
	logInfo.processName = row[4]
	logInfo.processCmd = row[5]
	logInfo.processId = pidVal
	logInfo.path = row[7]
	logInfo.status = row[8]
	logInfo.method = row[9]
	logInfo.sourceAddr = row[10]
	logInfo.sourcePort = sourcePortVal
	logInfo.destAddr = row[12]
	logInfo.destPort = destPortVal
	logInfo.bytesSent = bytesSentVal
	logInfo.protocol = row[15]

	return logInfo, nil
}

func splitCSVRow(rowText string) ([]string, error) {
	reader := csv.NewReader(strings.NewReader(rowText))
	fields, err := reader.Read()
	if err != nil && err != io.EOF {
		return nil, err
	}
	return fields, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

func writeLogEntry(activityLogFile *os.File, activityLogEntry *ActivityLogEntry) {
	logEntryCSV := strings.Join(serializeToCSV(activityLogEntry), ",")
	_, err := activityLogFile.WriteString(logEntryCSV + "\n")
	check(err)
}

func escapeCommandString(cmd string, args []string) string {
	consolidated := cmd + " " + strings.Join(args, " ")
	return escapeRawText(consolidated)
}

// Escapes commas and newlines
func escapeRawText(text string) string {
	return strings.ReplaceAll(strings.ReplaceAll(text, ",", "\\,"), "\n", "\\n")
}


// Helper for sending an HTTP/HTTPS request
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
			// Get the local address and port, as "100.100.100.100:1234" or "[a100:a200:a300:a400:a500:a600]:1234"
			localConnStr := connInfo.Conn.LocalAddr().String()
			fmt.Printf("Local address string is %s\n", localConnStr)
			if strings.Contains(localConnStr, "]:") {
				fmt.Println("Detected ipv6 local address")
				connStrPieces := strings.Split(localConnStr, "]:")
				sourceAddr = connStrPieces[0] + "]"
				sourcePort, err = strconv.Atoi(connStrPieces[1])
				check(err)
			} else {
				fmt.Println("Assuming ipv4 local address")
				connStrPieces := strings.Split(localConnStr, ":")
				sourceAddr = connStrPieces[0]
				sourcePort, err = strconv.Atoi(connStrPieces[1])
				check(err)
			}

			if err != nil {
				// TODO: What do we do if the port isn't present?
				fmt.Printf("Local host is addr %s port unknown\n", sourceAddr)
			} else {
				fmt.Printf("Local host is addr %s port %d\n", sourceAddr, sourcePort)
			}

			// TODO: Do the same for the remote address and port?
		},
		ConnectStart: func(network string, addr string) {},
		ConnectDone: func(network string, addr string, err error) {},
	}

	// Wrap the request with the tracer
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	// Emit the HTTP request
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

	// Print the response body and HTTP error code to the console, but do not add to activity log!
	fmt.Printf("Received HTTP(s) response code %d, and response body:\n=== START ===\n%s\n=== END ===\n\n", resp.StatusCode, responseBodyStr)

	// Return a success
	return makeSuccessResponse("sent", sourceAddr, sourcePort, int(req.ContentLength), path), nil
}

// TODO: Incorporate headers before sending a request?
func addHeadersAsNeeded(req *http.Request, headers any) {
	// panic("unimplemented")
}

// Injects the port number into the address
// Example: ('www.google.com/images', 80, 'https') -> 'https://www.google.com:80/images'
func injectPortIntoAddress(addr string, port int, protocol string) (string, error) {
	switch protocol {
	case "http", "https":
		u, err := url.Parse(protocol + "://" + addr)
		if err != nil {
			return "", fmt.Errorf("unable to parse address %s", addr)
		}

		hostWithPort := u.Hostname() + ":" + strconv.Itoa(port)
		fmt.Printf("Replacing '%s' with '%s' in '%s'...\n", u.Hostname(), hostWithPort, addr)
		newAddress := strings.Replace(addr, u.Hostname(), hostWithPort, 1)

		fmt.Printf("New URL: %s\n", newAddress)
		return newAddress, nil
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
func isCSVHeaderStr(line string) bool {
	return line == HeaderStr
}

// https://gist.github.com/lee8oi/ec404fa99ea0f6efd9d1
// https://stackoverflow.com/questions/78973708/how-can-i-scan-and-print-the-stdout-of-a-process-using-os-startprocess
func startProcess(cmd string, args []string) (*os.Process, context.CancelFunc, *os.ProcessState, error) {
	realCmd, err := exec.LookPath(cmd)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("unable to resolve path for %s: %v", cmd, err)
	}

	args = append([]string{realCmd}, args...)

	r, w, _ := os.Pipe()
	defer w.Close()
	defer r.Close()

	var procAttr os.ProcAttr
	procAttr.Files = []*os.File{os.Stdin, w, os.Stderr}

	lines := []string{}
	grCtx, grCancel := context.WithCancel(context.Background())
	go func(intCtx context.Context) {
		fmt.Printf("Reading from pipe...\n")
		rs := bufio.NewScanner(r)
		i := 0
		for rs.Scan() {
			select {
			case <- intCtx.Done():
				fmt.Printf("command exited, %d lines emitted\n", i)
				return
			default:
				i += 1
				text := rs.Text()
				fmt.Printf("%d: %s\n", i, text)
				lines = append(lines, text)
			}
		}
		fmt.Printf("Done reading from scanner\n")
	}(grCtx)

	fmt.Printf("Starting command %s with args %v\n", realCmd, args)
	p, err := os.StartProcess(realCmd, args, &procAttr)
	if err != nil {
		return nil, grCancel, nil, err
	}

	// Wait for process completion
	processState, err := p.Wait()
	if err != nil {
		return p, grCancel, nil, err
	}

	// Check the lines here? Thread-safe?
	fmt.Printf("Parsed lines: %#v\n", lines)

	return p, grCancel, processState, nil
}
