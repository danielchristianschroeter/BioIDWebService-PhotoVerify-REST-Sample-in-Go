package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Used for build version information
var version = "development"

// Global variables used for command line flags
var (
	BWSAppID         string
	BWSAppSecret     string
	photoPath        string
	image1Path       string
	image2Path       string
)

// Struct for the response from PhotoVerify API
type PhotoVerifyResponse struct {
	Success       bool   `json:"Success"`
	JobID         string `json:"JobID"`
	AccuracyLevel int    `json:"AccuracyLevel"`
	State         string `json:"State"`
	Samples       []struct {
		Errors []struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
			Details string `json:"Details"`
		} `json:"Errors"`
		EyeCenters struct {
			RightEyeX float64 `json:"RightEyeX"`
			RightEyeY float64 `json:"RightEyeY"`
			LeftEyeX  float64 `json:"LeftEyeX"`
			LeftEyeY  float64 `json:"LeftEyeY"`
		} `json:"EyeCenters"`
	} `json:"Samples"`
}

// Return an HTTP client with a timeout
func httpClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// Return the base64 encoded string for Basic Authentication
func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

// Send an HTTP request to the PhotoVerify API
func sendRequest(client *http.Client, method, BWSAppID, BWSAppSecret, photo, liveimage1, liveimage2 string) (int, []byte) {
	endpoint := "https://bws.bioid.com/extension/photoverify2"

	values := map[string]string{
		"idphoto":      photo,
		"liveimage1": liveimage1,
	}
	if liveimage2 != "" {
		values["liveimage2"] = liveimage2
	}
	jsonData, _ := json.Marshal(values)

	req, err := http.NewRequest(method, endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		log.Fatalf("Failed to create request: %v", err)
	}

	req.Header.Add("Content-Type", "application/json;charset=utf-8")
	req.Header.Add("Authorization", "Basic "+basicAuth(BWSAppID, BWSAppSecret))

	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("Error sending request to API endpoint: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Failed to read response body: %v", err)
	}

	return resp.StatusCode, body
}

// imageToBase64 converts an image file to a base64 encoded string
func imageToBase64(file string) (string, error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %v", file, err)
	}

	mimeType := http.DetectContentType(data)
	var base64Encoding string

	switch mimeType {
	case "image/jpeg":
		base64Encoding += "data:image/jpeg;base64,"
	case "image/png":
		base64Encoding += "data:image/png;base64,"
	default:
		return "", fmt.Errorf("unsupported mime type %v for file %v", mimeType, file)
	}

	base64Encoding += base64.StdEncoding.EncodeToString(data)
	return base64Encoding, nil
}

// Initialize command line flags
func init() {
	flag.StringVar(&BWSAppID, "BWSAppID", "", "BioIDWebService AppID")
	flag.StringVar(&BWSAppSecret, "BWSAppSecret", "", "BioIDWebService AppSecret")
	flag.StringVar(&photoPath, "photo", "", "Path to the reference photo image")
	flag.StringVar(&image1Path, "image1", "", "Path to the first live image")
	flag.StringVar(&image2Path, "image2", "", "Path to the second live image (optional)")
}

func main() {
	log.SetFlags(0)
	flag.Usage = func() {
		log.Println("BioIDWebService PhotoVerify REST Sample in Go. Version:", version)
		flag.PrintDefaults()
	}
	flag.Parse()

	if BWSAppID == "" || BWSAppSecret == "" || photoPath == "" || image1Path == "" {
		log.Fatal("Usage: -BWSAppID <BWSAppID> -BWSAppSecret <BWSAppSecret> -photo <photo> -image1 <image1> [-image2 <image2>]")
	}

	startTime := time.Now() // Record the start time of the entire process

	// Create a wait group for concurrent file reading and encoding
	var wg sync.WaitGroup
	wg.Add(2)
	if image2Path != "" {
		wg.Add(1)
	}

	var base64Photo, base64Image1, base64Image2 string
	var photoErr, image1Err, image2Err error

	// Concurrently read and encode photo
	go func() {
		defer wg.Done()
		base64Photo, photoErr = imageToBase64(photoPath)
	}()

	// Concurrently read and encode image1
	go func() {
		defer wg.Done()
		base64Image1, image1Err = imageToBase64(image1Path)
	}()

	// Concurrently read and encode image2 if it's provided
	if image2Path != "" {
		go func() {
			defer wg.Done()
			base64Image2, image2Err = imageToBase64(image2Path)
		}()
	}

	wg.Wait()

	// Check for errors during file reading and encoding
	if photoErr != nil {
		log.Fatalf("Failed to encode photo: %v", photoErr)
	}
	if image1Err != nil {
		log.Fatalf("Failed to encode image1: %v", image1Err)
	}
	if image2Err != nil {
		log.Fatalf("Failed to encode image2: %v", image2Err)
	}
	
	// Create HTTP client
	client := httpClient()

	statusCode, response := sendRequest(client, http.MethodPost, BWSAppID, BWSAppSecret, base64Photo, base64Image1, base64Image2)

	// Handle the response
	fmt.Printf("Total execution time: %v\n", time.Since(startTime)) // Print the total execution time
	fmt.Println()
	if statusCode == http.StatusOK {
		//log.Println(string(response))
		var result PhotoVerifyResponse
		if err := json.Unmarshal(response, &result); err != nil {
			log.Println("Failed to unmarshal JSON response.")
		}

		// Print specific fields
		fmt.Printf("Verification Status: %v\n", result.Success)
		fmt.Printf("Verification Accuracy Level: %v\n", result.AccuracyLevel)

		// Print Errors if any
		for i, sample := range result.Samples {
			if len(sample.Errors) > 0 {
				fmt.Printf("Sample %d Errors:\n", i+1)
				for _, err := range sample.Errors {
					fmt.Printf("\tCode: %s, Message: %s, Details: %s\n", err.Code, err.Message, err.Details)
				}
			}
		}
	} else {
		log.Fatalf("Received HTTP response code != 200: %d", statusCode)
	}
}