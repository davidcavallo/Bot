package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/anaskhan96/soup"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)
var telegramBotToken = os.Getenv("TRAF_FIC1")
var telegramAPIURL = "https://api.telegram.org/bot" + telegramBotToken

type Update struct {
	UpdateID int `json:"update_id"`
	Message  struct {
		MessageID int `json:"message_id"`
		From      struct {
			ID        int    `json:"id"`
			IsBot     bool   `json:"is_bot"`
			FirstName string `json:"first_name"`
			Username  string `json:"username"`
			Language  string `json:"language_code"`
		} `json:"from"`
		Chat struct {
			ID   int    `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		Date     int    `json:"date"`
		Text     string `json:"text"`
		Entities []struct {
			Offset int    `json:"offset"`
			Length int    `json:"length"`
			Type   string `json:"type"`
		} `json:"entities"`
	} `json:"message"`
}

func main() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)

	// Root endpoint for health check
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Server is running"))
	})

	// Webhook endpoint
	r.Post("/", handleTelegramWebhook)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("Server is listening on port", port)
	log.Fatal(http.ListenAndServe(":"+port, r))
}

func handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	log.Println("Received a webhook request")
	var update Update

	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		log.Printf("Could not decode update: %v", err)
		return
	}

	log.Printf("Received message: %s", update.Message.Text)

	if update.Message.Text == "" {
		log.Println("No text found in the message")
		return
	}

	website := update.Message.Text
	log.Printf("Fetching info for website: %s", website)
	go func() {
		responseMessage := fetchWebsiteInfoWithRetry(website)
		sendMessage(update.Message.Chat.ID, responseMessage)
	}()
}

func fetchWebsiteInfoWithRetry(website string) string {
	const maxRetries = 3
	var responseMessage string

	for i := 0; i < maxRetries; i++ {
		responseMessage = fetchWebsiteInfo(website)
		if !strings.Contains(responseMessage, "Could not find number of visitors") && !strings.Contains(responseMessage, "403") {
			return responseMessage
		}
		log.Printf("Retry %d: Could not fetch the information, retrying...", i+1)
		time.Sleep(2 * time.Second) // Increased wait time to 2 seconds
	}

	return responseMessage
}

func fetchWebsiteInfo(website string) string {
    url := "https://www.similarweb.com/website/" + website + "/competitors/"
    req, err := http.NewRequest(http.MethodGet, url, nil)
    if err != nil {
        log.Printf("Could not create request: %v", err)
        return "An error occurred while processing your request."
    }
    req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:134.0) Gecko/20100101 Firefox/134.0")
    req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")	
    req.Header.Set("Accept-Language", "en-US,en;q=0.5")
    req.Header.Set("Accept-Encoding", "identity")
    req.Header.Set("Cookie", "bm_s=YAAQrbsuF0IqWpCUAQAAA7c9lAKVNQ4KrNqOAfwpMaAVsRdDof3QXHJFei5G4Vw9t5KCCcrHSf6gc8WmYyBejhQux/Ehjcqt56JVompMmqw8s13sD6Cil5OEdnVlS6cQxTsUV7i+8SRFUnSWuuvrdtbd4mRdbuJQquFhR1CdGWupT7+P883lhUx28c/fsUGj3IzX3ly2XGdHkrR0vlB22Ho3ZCsLRVp13fTCDrJrrmqDjpJ4c5QhuveId1Chc740QjDUhhQeN7JYp//Aj2Vz5pG25DVU9scZapzbPcwSVHYwj1ktyGrrRi/sYiFQLgypNe0VB1xeXMezD0/V9Unr/G9UXyOZsim73wNo+EaEKcEq8+1SovTUs0qOrnA2P2H0TXGZZBUB6C+I2VqgWYLq4YK/lIY+ijo4IyCL7cCgGpWEdGepIK7HwJIW8DBKNPPZso93hDgUBlkhxiyXl+elEoirJ5qnPRaGm7eZlDhGEFU=")

    client := &http.Client{Timeout: 10 * time.Second}
    resp, err := client.Do(req)
    if err != nil {
        log.Printf("Could not fetch website info: %v", err)
        return "An error occurred while fetching the website."
    }
    defer resp.Body.Close()
	
	// Read from the decompressed reader
	
    if resp.StatusCode != http.StatusOK {
        log.Printf("Unexpected status code: %d", resp.StatusCode)
        if resp.StatusCode == http.StatusForbidden {
            return "403 Forbidden: Access is denied."
        }
        return fmt.Sprintf("Failed with status code %d.", resp.StatusCode)
    }

    buf, err := io.ReadAll(resp.Body)
    if err != nil || len(buf) == 0 {
        log.Printf("Could not read response body: %v", err)
        return "Could not process the response from the website."
    }

    doc := soup.HTMLParse(string(buf))

    var sb strings.Builder

    // Add website name to the output
    sb.WriteString(fmt.Sprintf("Website: %s\n", website))

    // Extract number of visitors
    maintraff := doc.Find("p", "class", "engagement-list__item-value")
    if maintraff.Error != nil || maintraff.Pointer == nil {
        log.Printf("Error finding main traffic element: %v", maintraff.Error)
        sb.WriteString("Number of Visitors: Not found\n")
    } else {
        sb.WriteString(fmt.Sprintf("Number of Visitors: %s\n", maintraff.Text()))
    }

    // Extract website description
    description := doc.Find("div", "class", "wa-overview__description")
    if description.Error == nil && description.Pointer != nil {
        sb.WriteString(fmt.Sprintf("Description: %s\n", description.FullText()))
    } else {
        log.Printf("Description not found: %v", description.Error)
	log.Printf("Full response body: %s", string(buf))
        sb.WriteString("Description: Not found\n")
    }

    // Extract competitors
    divs := doc.FindAll("div", "class", "wa-competitors-card")
    if len(divs) == 0 {
        sb.WriteString("\nNo competitors found.\n")
    } else {
        sb.WriteString("\nCompetitors:\n")
        for _, div := range divs {
            // Extract competitor title
            title := div.Find("a", "class", "wa-competitors-card__website-title")
            if title.Error == nil && title.Pointer != nil {
                sb.WriteString(fmt.Sprintf("Title: %s\n", title.Text()))
            } else {
                sb.WriteString("Title: Not found\n")
            }

            // Extract competitor description
            descr := div.Find("p", "class", "wa-competitors-card__website-description")
            if descr.Error == nil && descr.Pointer != nil {
                sb.WriteString(fmt.Sprintf("Description: %s\n", descr.Text()))
            } else {
                sb.WriteString("Description: Not found\n")
            }

            // Extract competitor traffic
            traff := div.Find("p", "class", "engagement-list__item-value")
            if traff.Error == nil && traff.Pointer != nil {
                sb.WriteString(fmt.Sprintf("Traffic: %s\n", traff.Text()))
            } else {
                sb.WriteString("Traffic: Not found\n")
            }

            sb.WriteString("\n")
        }
    }

    return sb.String()
}

func sendMessage(chatID int, text string) {
	url := fmt.Sprintf("%s/sendMessage", telegramAPIURL)
	data := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	payload, err := json.Marshal(data)
	if err != nil {
		log.Printf("Could not marshal payload: %v", err)
		return
	}

	resp, err := http.Post(url, "application/json", strings.NewReader(string(payload)))
	if err != nil {
		log.Printf("Could not send message: %v", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Could not read response body: %v", err)
		return
	}

	log.Printf("Sent message response: %s", string(body))
}
