# Test Strategy

For each test case, in your local terminal, run `go run . [args]` and verify the output.

By default, activity log output is appended to `./activity-log.csv`. A header row indicates what each field represents. Tailing this file or having it open in an IDE alongside your output will allow you to observe new records being added in real-time.

Fields:
- timestamp         Timestamp of the event (RFC3338)
- activity          Which command is performed (execute, create, update, delete, send)
- os                The OS string the user is on (windows, darwin, linux)
- username          The local username of the user (ie. WINDOWSBOX\Bobby on Windows, bobby on Mac/Linux)
- processName       The name of the process being run
- processCmd        The full command and arguments being executed (or pseudo-command, for file and network operations)
- pid               The PID of the process performing the activity (either this program's PID or the PID of the spawned process, for execute)
- path              The file path or full URI with arguments (file and network operations only)
- status            A status code indicating outcome of the activity (or the HTTP response's status code, for send)
- method            (send only) The HTTP(S) method (ie. GET, POST, DELETE, etc.)
- sourceAddr        (send only) The source IP address (IPv4/v6) of the sender
- sourcePort        (send only) The source TCP/UDP port of the sender
- destAddr          (send only) The destination IP address (IPv4/v6) of the sender
- destPort          (send only) The destination TCP/UDP port of the sender
- bytesSent         (send only) The number of bytes sent in the body of the request
- protocol          (send only) The protocol (supports http or https, default http)

# Test Cases

(Execute)
1. go run . execute go version
    Output: "go version go1.23.2 <os>/<arch>"
    Activity Log:
        activity: execute
        processCmd: go version
2. go run . execute nonexistent-program
    Output: "panic: exec: "nonexistent-program": executable file not found in PATH"
    Activity Log: none
(Create)
3. go run . create ./test.txt
    Output: "0 bytes written to new file ./test.txt"
    Activity Log:
        activity: create
        processCmd: create ./test.txt
        status: created
4. go run . create ./README.md
    Output: "File ./README.md already exists, unable to write!"
    Activity Log:
        activity: create
        processCmd: create ./README.md
        status: exists
5. go run . create /root
    Output: "Error: open /root: Access is denied."
    Activity Log:
        activity: create
        processCmd: create /root
        status: error
6. go run . create
    Output: "panic: not enough arguments for create! Args: []"
    Activity Log: none
7. go run . create ./test.txt "Hello World!"
    Output: "12 bytes written to new file ./test.txt"
    Activity Log:
        activity: create
        processCmd: create ./test.txt Hello World!
        status: created
(Update)
8. go run . update ./test.txt
    Output: "0 bytes written to existing file ./test.txt"
    Activity Log:
        activity: update
        processCmd: update ./test.txt
        status: updated
9. go run . update ./test.txt "Hello World!"
    Output: "12 bytes written to existing file ./test.txt"
    Activity Log:
        activity: update
        processCmd: update ./test.txt Hello World!
        status: updated
10. go run . update /root
    Output: "Error: open /root: Access is denied."
    Activity Log:
        activity: update
        processCmd: update /root
        status: error
11. go run . update ./nonexistent-file "Missing?"
    Output: "File ./nonexistent-file not found for updating!"
    Activity Log:
        activity: update
        processCmd: update ./nonexistent-file Missing?
        status: not_found
(Delete)
12. go run . delete ./test.txt
    Output "File ./test.txt deleted"
    Activity Log:
        activity: delete
        processCmd: delete ./test.txt
        status: deleted
13. go run . delete ./nonexistent-file
    Output: "File ./nonexistent-file not found for deleting!"
    Activity Log:
        activity: delete
        processCmd: delete ./test.txt
        status: not_found
14. (Windows) go run . delete C:\Windows\system.ini
    (Mac/Linux) go run . delete /root
    Output: none
    Activity Log:
        activity: delete
        processCmd: delete C:\Windows\system.ini (Windows)
                    delete /root (Mac/Linux)
        status: error
15. go run . delete
    Output: "panic: not enough arguments for delete! Args: []"
    Activity Log: none
(Send)
16. go run . send
    Output: "panic: not enough arguments for send! Args: []"
    Activity Log: none
17. go run . send GET
    Output: "panic: not enough arguments for send! Args: [GET]"
    Activity Log: none
18. go run . send GET www.google.com
    Output: "Sending 0 bytes of data to GET www.google.com (port 80) using protocol http..."
    Activity Log:
        activity: send
        processCmd: send GET www.google.com
        path: http://www.google.com:80
        status: sent
        bytesSent: 0
        method: GET
        destAddr: www.google.com
        destPort: 80
        protocol: http
19. go run . send GET www.google.com 80
    Output: "Sending 0 bytes of data to GET www.google.com (port 80) using protocol http..."
    Activity Log:
        activity: send
        processCmd: send GET www.google.com
        path: http://www.google.com:80
        status: sent
        bytesSent: 0
        method: GET
        destAddr: www.google.com
        destPort: 80
        protocol: http
20. go run . send GET www.google.com 80 http
    Output: "Sending 0 bytes of data to GET www.google.com (port 80) using protocol http..."
    Activity Log:
        activity: send
        processCmd: send GET www.google.com
        path: http://www.google.com:80
        status: sent
        bytesSent: 0
        method: GET
        destAddr: www.google.com
        destPort: 80
        protocol: http
21. go run . send POST www.postman-echo.com/post 443 https "Hello World!"
    Output:
        Received HTTP(s) response code 200, and response body:
        === START ===
        {
            "args": {},
            "data": "Hello World!",
            // ...
        }
    Activity Log:
        activity: send
        processCmd: send POST www.postman-echo.com/post 443 https Hello World!
        path: https://www.postman-echo.com:443/post
        status: sent
        bytesSent: 12
        method: POST
        destAddr: www.postman-echo.com/post
        destPort: 443
        protocol: https
22. go run . send GET www.google.com 443 http
    Output: "Sending 0 bytes of data to GET www.google.com (port 443) using protocol http..." (NOTE: port 443 is HTTPS-only, so this fails with a protocol error!)
    Activity Log:
        activity: send
        processCmd: send GET www.google.com 443 http
        path: http://www.google.com:443
        status: error
        bytesSent: 0
        method: GET
        destAddr: www.google.com
        destPort: 443
        protocol: http
23. go run . send GET INVALID_URL
    Output: none (path is invalid)
    Activity Log:
        activity: send
        processCmd: send GET INVALID_URL
        path: http://INVALID_URL:80
        status: error
        bytesSent: 0
        method: GET
        destAddr: INVALID_URL
        destPort: 80
        protocol: http
24. go run . send GET www.google.com 65536
    Output: none (port number is invalid)
    Activity Log:
        activity: send
        processCmd: send GET www.google.com 65536
        path: http://www.google.com:65536
        status: error
        bytesSent: 0
        method: GET
        destAddr: www.google.com
        destPort: 65536
        protocol: http