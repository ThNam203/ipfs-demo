package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	ipfslite "ipfs-demo/ipfs"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/ipfs/go-cid"
	"github.com/rs/cors"
)

type FileInfo struct {
	Filename string `json:"filename"`
	CID      string `json:"cid"`
	Size     int64  `json:"size"`
	Type     string `json:"type"`
}

var (
	ipfsNode      *ipfslite.Peer
	upgrader      = websocket.Upgrader{}
	clients       = make(map[*websocket.Conn]bool) // Connected clients
	broadcastChan = make(chan FileInfo)            // Channel for broadcasting file info
	mu            sync.Mutex                       // To manage access to clients map
)

func getFileFromNode(w http.ResponseWriter, r *http.Request) {
	fileCid := r.PathValue("fileCid")
	c, _ := cid.Decode(fileCid)
	rsc, err := ipfsNode.GetFile(r.Context(), c)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	defer rsc.Close()

	// Set headers to force download
	w.Header().Set("Content-Disposition", "attachment; filename="+fileCid)
	w.Header().Set("Content-Type", "application/octet-stream")

	// Stream the file to the client
	_, err = io.Copy(w, rsc)
	if err != nil {
		http.Error(w, "Failed to write file to response", http.StatusInternalServerError)
	}
}

func getFileInfosHandler(w http.ResponseWriter, r *http.Request) {
	file, err := os.Open("uploaded_files.txt")
	if err != nil {
		http.Error(w, "Could not open the file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	var fileInfos []FileInfo
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ", ")
		if len(parts) != 4 {
			continue // Skip malformed lines
		}

		var filename, cid, fileType string
		var size int64

		for _, part := range parts {
			kv := strings.SplitN(part, ": ", 2)
			if len(kv) != 2 {
				continue
			}
			switch kv[0] {
			case "Filename":
				filename = kv[1]
			case "CID":
				cid = kv[1]
			case "Size":
				fmt.Sscanf(kv[1], "%d", &size)
			case "Type":
				fileType = kv[1]
			}
		}

		fileInfo := FileInfo{
			Filename: filename,
			CID:      cid,
			Size:     size,
			Type:     fileType,
		}
		fileInfos = append(fileInfos, fileInfo)
	}

	if err := scanner.Err(); err != nil {
		http.Error(w, "Error reading the file", http.StatusInternalServerError)
		return
	}

	// Set the content type to application/json
	w.Header().Set("Content-Type", "application/json")

	// Return the file information as JSON
	json.NewEncoder(w).Encode(fileInfos)
}

func logFileInfo(filename, cid string, size int64, fileType string) error {
	file, err := os.OpenFile("uploaded_files.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = fmt.Fprintf(file, "Filename: %s, CID: %s, Size: %d bytes, Type: %s\n", filename, cid, size, fileType)
	return err
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	// Parse the multipart form to handle file upload
	err := r.ParseMultipartForm(10 << 20) // 10MB limit for file size
	if err != nil {
		http.Error(w, "Could not parse multipart form", http.StatusBadRequest)
		return
	}

	// Get the files from the form data
	files := r.MultipartForm.File["files"]
	if files == nil {
		http.Error(w, "No files uploaded", http.StatusBadRequest)
		return
	}

	var fileInfos []FileInfo

	for _, fileHeader := range files {
		// Open the uploaded file
		file, err := fileHeader.Open()
		if err != nil {
			http.Error(w, "Error retrieving file from form", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Save the file locally
		// filePath := filepath.Join("./uploads", fileHeader.Filename)
		// tempFile, err := os.Create(filePath)
		// if err != nil {
		// 	http.Error(w, "Error saving file locally", http.StatusInternalServerError)
		// 	return
		// }
		// defer tempFile.Close()

		// Save file to IPFS
		ipldNode, err := ipfsNode.AddFile(r.Context(), file)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error saving file to IPFS: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		fmt.Printf("saved a file with cid: %s", ipldNode.Cid().String())

		// Gather file information
		fileSize := fileHeader.Size
		fileType := fileHeader.Header.Get("Content-Type")

		// Create FileInfo struct
		fileInfo := FileInfo{
			Filename: fileHeader.Filename,
			CID:      ipldNode.Cid().String(),
			Size:     fileSize,
			Type:     fileType,
		}

		fileInfos = append(fileInfos, fileInfo)

		// Log file info to text file
		if err := logFileInfo(fileHeader.Filename, ipldNode.Cid().String(), fileSize, fileType); err != nil {
			fmt.Printf("error while logging file info: %s\n", err.Error())
			http.Error(w, "Error logging file info", http.StatusInternalServerError)
			return
		}

		broadcastChan <- fileInfo
	}

	// Set the content type to application/json
	w.Header().Set("Content-Type", "application/json")

	// Return the file information as JSON
	json.NewEncoder(w).Encode(fileInfos)
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Could not upgrade to websocket", http.StatusInternalServerError)
		return
	}
	fmt.Println("a websocket connected")
	defer conn.Close()

	mu.Lock()
	clients[conn] = true
	mu.Unlock()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			mu.Lock()
			delete(clients, conn)
			mu.Unlock()
			fmt.Println("a websocket disconnected")
			break
		}
	}
}

// Broadcast file information to all clients
func broadcastFiles() {
	for {
		fileInfo := <-broadcastChan
		mu.Lock()
		for conn := range clients {
			err := conn.WriteJSON(fileInfo)
			if err != nil {
				conn.Close()
				delete(clients, conn)
			}
		}
		mu.Unlock()
	}
}

func setUpFolders() {
	// Erase data on start
	os.RemoveAll("uploaded_files.txt")
	os.RemoveAll("./uploads")

	err := os.MkdirAll("./uploads", os.ModePerm)
	if err != nil {
		fmt.Printf("Error creating uploads directory: %s", err.Error())
		return
	}

	f, err := os.OpenFile("./uploaded_files.txt", os.O_CREATE, os.ModePerm)
	if err != nil {
		fmt.Printf("Error creating uploaded_files: %s", err.Error())
		return
	}
	f.Close()
}

func main() {
	setUpFolders()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ds := ipfslite.NewInMemoryDatastore()
	host, dht, err := ipfslite.SetupLibp2p(ctx, ds)
	if err != nil {
		panic(err)
	}

	ipfsNode, err = ipfslite.New(ctx, ds, host, dht)
	if err != nil {
		panic(err)
	}

	fmt.Printf("ipfs node run with id (%s), addr: %v\n", ipfsNode.GetHost().ID(), ipfsNode.GetHost().Addrs())
	go ipfsNode.Bootstrap(ipfslite.DefaultBootstrapPeers())

	go broadcastFiles()

	// Set up the HTTP server and upload route
	mux := http.NewServeMux()
	mux.HandleFunc("/upload", uploadHandler)
	mux.HandleFunc("/files", getFileInfosHandler)
	mux.HandleFunc("/files/{fileCid}", getFileFromNode)
	mux.HandleFunc("/socket", wsHandler)
	handler := cors.Default().Handler(mux)

	fmt.Println("Starting server on :8000...")
	if err := http.ListenAndServe(":8000", handler); err != nil {
		fmt.Printf("Error starting server: %s", err.Error())
	}
}
