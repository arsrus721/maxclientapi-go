package main

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"your_project/maxclientapi" // Replace with your package path
)

func main() {
	// Authorization token for API (identifies your client)
	token := "YOUR TOKEN HERE"

	// Unique device ID (used to identify your client instance)
	deviceID := "DEVICE ID"

	// Chat ID where the message will be sent
	chatID := 123456 // CHAT ID without quotes

	// Path to the file that will be uploaded
	filePath := "/path/to/your/file"

	// Create a Max API client using the provided token and device ID
	client := maxclientapi.NewChatClient(
		token,
		deviceID,
		maxclientapi.WithDebug(false),           // Enable/disable debug mode
		maxclientapi.WithAllowReconnect(true),   // Auto-reconnect on disconnect
	)

	// Connect to the server
	err := client.Connect()
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}

	// Start keepalive (sends periodic pings to keep the connection active)
	client.StartKeepalive(25 * time.Second)

	// Send a text message to the chat
	client.SendMessage(chatID, "Hello")

	// Subscribe to chat updates (so the client receives incoming messages)
	client.SubscribeChat(chatID)

	// Request an upload URL for future file sending
	client.RequestURLToSendFile(1)

	// Variables to store tokens
	var tokenFile string
	var fileID interface{}

	// Main loop — continuously listens for new incoming messages
	for {
		msg := client.GetMessageBlocking() // Wait for a new message (blocking call)
		if msg == nil {
			continue // Skip iteration if no message received
		}

		msgType, ok := msg["type"].(string)
		if !ok {
			continue
		}

		switch msgType {
		// If the message is a download link
		case "download_link":
			fmt.Printf("External: %v\n", msg["EXTERNAL"])
			fmt.Printf("video_url: %v\n", msg["video_url"])
			fmt.Printf("raw: %v\n", msg["raw"])

		// If the message contains a photo
		case "photo":
			fmt.Println("chat_id:", msg["chat_id"])
			fmt.Println("sender:", msg["sender"])
			fmt.Println("id:", msg["id"])
			fmt.Println("time:", msg["time"])
			fmt.Println("utype:", msg["utype"])
			fmt.Println("baseUrl:", msg["baseUrl"])
			fmt.Println("previewData:", msg["previewData"])
			fmt.Println("photoToken:", msg["photoToken"])
			fmt.Println("width:", msg["width"])
			fmt.Println("photoId:", msg["photoId"])
			fmt.Println("height:", msg["height"])
			fmt.Println("raw:", msg["raw"])

		// If the message contains a photo with text
		case "photo_with_text":
			fmt.Println("text:", msg["text"])
			fmt.Println("chat_id:", msg["chat_id"])
			fmt.Println("sender:", msg["sender"])
			fmt.Println("id:", msg["id"])
			fmt.Println("time:", msg["time"])
			fmt.Println("utype:", msg["utype"])
			fmt.Println("baseUrl:", msg["baseUrl"])
			fmt.Println("previewData:", msg["previewData"])
			fmt.Println("photoToken:", msg["photoToken"])
			fmt.Println("width:", msg["width"])
			fmt.Println("photoId:", msg["photoId"])
			fmt.Println("height:", msg["height"])
			fmt.Println("raw:", msg["raw"])

		// If the message contains a video
		case "video":
			fmt.Printf("chat_id: %v\n", msg["chat_id"])
			fmt.Printf("sender: %v\n", msg["sender"])
			fmt.Printf("thumbnail: %v\n", msg["thumbnail"])
			fmt.Printf("duration: %v\n", msg["duration"])
			fmt.Printf("width: %v\n", msg["width"])
			fmt.Printf("videoId: %v\n", msg["videoId"])
			fmt.Printf("token: %v\n", msg["token"])
			fmt.Printf("height: %v\n", msg["height"])
			fmt.Printf("raw: %v\n", msg["raw"])
			fmt.Printf("id: %v\n", msg["id"])
			fmt.Printf("time: %v\n", msg["time"])
			fmt.Printf("utype: %v\n", msg["utype"])
			fmt.Printf("prevMessageId: %v\n", msg["prevMessageId"])

			// Save the video token
			tokenFile = fmt.Sprintf("%v", msg["token"])

			// Request the video URL for downloading
			client.GetVideoURL(msg["videoId"], chatID, msg["id"])

		// If the message contains a video with text
		case "video_with_text":
			fmt.Printf("text: %v\n", msg["text"])
			fmt.Printf("chat_id: %v\n", msg["chat_id"])
			fmt.Printf("sender: %v\n", msg["sender"])
			fmt.Printf("thumbnail: %v\n", msg["thumbnail"])
			fmt.Printf("duration: %v\n", msg["duration"])
			fmt.Printf("width: %v\n", msg["width"])
			fmt.Printf("videoId: %v\n", msg["videoId"])
			fmt.Printf("token: %v\n", msg["token"])
			fmt.Printf("height: %v\n", msg["height"])
			fmt.Printf("raw: %v\n", msg["raw"])
			fmt.Printf("id: %v\n", msg["id"])
			fmt.Printf("time: %v\n", msg["time"])
			fmt.Printf("utype: %v\n", msg["utype"])
			fmt.Printf("prevMessageId: %v\n", msg["prevMessageId"])

			// Save the video token
			tokenFile = fmt.Sprintf("%v", msg["token"])

			// Request the video URL for downloading
			client.GetVideoURL(msg["videoId"], chatID, msg["id"])

		// If the message is a regular text message
		case "text":
			fmt.Println("text:", msg["text"])
			fmt.Println("chat_id:", msg["chat_id"])
			fmt.Println("sender:", msg["sender"])
			fmt.Println("id:", msg["id"])
			fmt.Println("time:", msg["time"])
			fmt.Println("utype:", msg["utype"])
			fmt.Println("prevMessageId:", msg["prevMessageId"])

		// If the server provides a URL for file upload
		case "url_upload":
			fmt.Printf("url: %v\n", msg["url"])
			fmt.Printf("token: %v\n", msg["token"])
			fmt.Printf("fileId: %v\n", msg["fileId"])

			tokenFile = fmt.Sprintf("%v", msg["token"]) // Save upload token
			fileID = msg["fileId"]                      // File ID assigned by the server

			// Upload the file to the provided URL
			uploadURL := fmt.Sprintf("%v", msg["url"])
			err := uploadFile(uploadURL, filePath, tokenFile)
			if err != nil {
				log.Printf("Failed to upload file: %v", err)
				continue
			}

			// After the file is uploaded, send it to the chat
			client.SendFile(chatID, fileID)

		// If the message is a link
		case "link":
			fmt.Printf("text: %v\n", msg["text"])
			fmt.Printf("chat_id: %v\n", msg["chat_id"])
			fmt.Printf("sender: %v\n", msg["sender"])

		default:
			fmt.Printf("Unknown message type: %s\n", msgType)
		}
	}
}

// uploadFile uploads a file to the specified URL with authorization
func uploadFile(url, filePath, token string) error {
	// Open the local file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Create a buffer for the multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Create a form file field
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}

	// Copy the file content to the form
	_, err = io.Copy(part, file)
	if err != nil {
		return fmt.Errorf("failed to copy file content: %w", err)
	}

	// Close the writer to finalize the form
	err = writer.Close()
	if err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	// Create an HTTP request
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	// Send the request
	client := &http.Client{
		Timeout: 60 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Check the response status
	fmt.Printf("Response status code: %d\n", resp.StatusCode)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}