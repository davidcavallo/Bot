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
		port = "80"
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
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:130.0) Gecko/20100101 Firefox/130.0")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		log.Printf("Could not fetch website info: %v (status code: %d)", err, resp.StatusCode)
		if resp.StatusCode == http.StatusForbidden {
			return "403 Forbidden: Access is denied."
		}
		return "An error occurred while processing your request."
	}
	defer resp.Body.Close()

	buf, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Could not read response body: %v", err)
		return "An error occurred while processing your request."
	}
	doc := soup.HTMLParse(string(buf))

	// Find the number of visitors
	maintraff := doc.Find("p", "class", "engagement-list__item-value")
	if maintraff.Error != nil {
		log.Printf("Could not find number of visitors: %v", maintraff.Error)
		return "Could not find number of visitors."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Website: %s\n", website))
	sb.WriteString(fmt.Sprintf("Number of Visitors: %s\n", maintraff.Text()))

	// Find the description
	description := doc.Find("div", "class", "wa-overview__description")
	if description.Error == nil {
		sb.WriteString(fmt.Sprintf("Description: %s\n", description.FullText()))
	} else {
		log.Printf("Could not find description: %v", description.Error)
		sb.WriteString("Description not found\n")
	}

	// Find competitors
	divs := doc.FindAll("div", "class", "wa-competitors-card")
	if len(divs) == 0 {
		sb.WriteString("\nNo competitors found")
	} else {
		sb.WriteString("\nCompetitors:\n")
		for _, div := range divs {
			title := div.Find("a", "class", "wa-competitors-card__website-title")
			if title.Error == nil {
				sb.WriteString(fmt.Sprintf("Title: %s\n", title.Text()))
				descr := div.Find("p", "class", "wa-competitors-card__website-description")
				if descr.Error == nil {
					sb.WriteString(fmt.Sprintf("Description: %s\n", descr.Text()))
				} else {
					sb.WriteString("Description not found\n")
				}
				traff := div.Find("p", "class", "engagement-list__item-value")
				if traff.Error == nil {
					sb.WriteString(fmt.Sprintf("Traffic: %s\n", traff.Text()))
				} else {
					sb.WriteString("Traffic not found\n")
				}
				sb.WriteString("\n")
			} else {
				sb.WriteString("Title not found\n")
			}
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
