package main

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log"
	"m3u-stream-merger/database"
	"m3u-stream-merger/utils"
	"net/http"
	"strconv"
	"strings"
	"syscall"

	"github.com/redis/go-redis/v9"
)

func mp4Handler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	// Create a context with cancellation capability
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	// Log the incoming request
	log.Printf("Received request from %s for URL: %s\n", r.RemoteAddr, r.URL.Path)

	// Extract the m3u ID from the URL path
	m3uID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/stream/"), ".mp4")
	if m3uID == "" {
		http.NotFound(w, r)
		return
	}

	streamName := utils.GetStreamName(m3uID)
	if streamName == "" {
		http.NotFound(w, r)
		return
	}

	stream, err := database.GetStreamByTitle(db, streamName)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// You can modify the response header as needed
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	var resp *http.Response
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()

	// Iterate through the streams and select one based on concurrency and availability
	var selectedUrl *database.StreamURL

	// Concurrency check mode
	for _, url := range stream.URLs {
		if checkConcurrency(ctx, url.Content, url.MaxConcurrency) {
			continue // Skip this stream if concurrency limit reached
		}

		resp, err = http.Get(url.Content)
		if err == nil {
			selectedUrl = &url
			updateConcurrency(ctx, url.Content, true)
			break
		}

		// Log the error
		log.Printf("Error fetching MP4 stream (concurrency check mode): %s\n", err.Error())
	}

	if selectedUrl == nil {
		// Connection check mode
		for _, url := range stream.URLs {
			resp, err = http.Get(url.Content)
			if err == nil {
				selectedUrl = &url
				updateConcurrency(ctx, url.Content, true)
				break
			}
			// Log the error
			log.Printf("Error fetching MP4 stream (connection check mode): %s\n", err.Error())
		}

		if resp == nil {
			// Log the error
			log.Println("Error fetching MP4 stream. Exhausted all streams.")
			// Check if the connection is still open before writing to the response
			select {
			case <-r.Context().Done():
				// Connection closed, handle accordingly
				log.Println("Client disconnected")
				return
			default:
				// Connection still open, proceed with writing to the response
				http.Error(w, "Error fetching MP4 stream. Exhausted all streams.", http.StatusInternalServerError)
				return
			}
		}
	}

	// Log the successful response
	log.Printf("Sent MP4 stream to %s\n", r.RemoteAddr)

	// Check if the connection is still open before copying the MP4 stream to the response
	select {
	case <-r.Context().Done():
		// Connection closed, handle accordingly
		log.Println("Client disconnected after fetching MP4 stream")
		updateConcurrency(ctx, selectedUrl.Content, false)
		return
	default:
		// Connection still open, proceed with writing to the response
		_, err := io.Copy(w, resp.Body)
		if err != nil {
			// Log the error
			if errors.Is(err, syscall.EPIPE) {
				log.Println("Client disconnected after fetching MP4 stream")
				updateConcurrency(ctx, selectedUrl.Content, false)
			} else {
				log.Printf("Error copying MP4 stream to response: %s\n", err.Error())
			}
			return
		}
	}
}

func checkConcurrency(ctx context.Context, url string, maxConcurrency int) bool {
	redisClient := database.InitializeRedis()
	val, err := redisClient.Get(ctx, url).Result()
	if err == redis.Nil {
		return false // Key does not exist
	} else if err != nil {
		log.Printf("Error checking concurrency: %s\n", err.Error())
		return false // Error occurred, treat as concurrency not reached
	}

	count, _ := strconv.Atoi(val)
	return count >= maxConcurrency
}

func updateConcurrency(ctx context.Context, url string, incr bool) {
	redisClient := database.InitializeRedis()
	var err error
	if incr {
		err = redisClient.Incr(ctx, url).Err()
	} else {
		err = redisClient.Decr(ctx, url).Err()
	}
	if err != nil {
		log.Printf("Error updating concurrency: %s\n", err.Error())
	}
}