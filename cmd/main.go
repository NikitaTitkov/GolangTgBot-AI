package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/subosito/gotenv"
)

const (
	baseURL                   = "http://localhost:8001"
	createPostfix             = "/create"
	waitingTextMessage        = "waiting_for_text"
	waitingDescriptionMessage = "waiting_for_description"
)

// Promt struct to hold the description and text of a prompt.
type Promt struct {
	Description string `json:"description"`
	Text        string `json:"text"`
}

// SendPromt - function to send a prompt to the server.
func SendPromt(p *Promt) (string, error) {
	// Marshal the prompt to JSON.
	data, err := json.Marshal(p)
	if err != nil {
		return "", err
	}

	// Send a POST request to the server with the JSON data.
	resp, err := http.Post(baseURL+createPostfix, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return "", err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Println("Ошибка при закрытии Body:", err)
		}
	}()

	// Check the status code of the server's response.
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned non-OK status: %s", resp.Status)
	}

	// Create a file to save the server's response.
	mp3FileName := "response.mp3"
	file, err := os.Create(mp3FileName)
	if err != nil {
		return "", err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Println("Ошибка при закрытии файла:", err)
		}
	}()

	// Copy the server's response into the file.
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return "", err
	}
	return mp3FileName, nil
}

func main() {
	if err := gotenv.Load(".env"); err != nil {
		log.Fatal("Error loading .env file: ", err)
	}
	// Get the bot token from the environment variable.
	botToken := os.Getenv("TG_KEY")
	fmt.Println(botToken)

	// Create a new bot with the obtained token.
	bot, err := tgbotapi.NewBotAPI(botToken)
	if err != nil {
		log.Fatalf("Error creating bot: %v", err)
	}

	// Get the updates from the bot.
	updates, err := bot.GetUpdatesChan(tgbotapi.NewUpdate(0))
	if err != nil {
		log.Fatalf("Error getting updates: %v", err)
	}

	// Create maps to store the user states and their inputs.
	userStates := make(map[int64]string)
	userInputs := make(map[int64]Promt)

	// Create a channel to pass updates from the bot.
	messageChan := make(chan tgbotapi.Update)

	// A goroutine to handle updates.
	go func() {
		for update := range messageChan {
			if update.Message == nil {
				continue
			}

			chatID := update.Message.Chat.ID
			msgText := update.Message.Text

			switch userStates[chatID] {
			case "":
				if msgText == "/start" {
					_, err := bot.Send(tgbotapi.NewMessage(chatID, "Hello! Enter a description:"))
					if err != nil {
						log.Fatal("Error sending message: ", err)
					}
					userStates[chatID] = waitingDescriptionMessage
				} else {
					_, err := bot.Send(tgbotapi.NewMessage(chatID, "Unknown command. Use /start."))
					if err != nil {
						log.Fatal("Error sending message: ", err)
					}

				}
			case "waiting_for_description":
				userInputs[chatID] = Promt{Description: msgText}
				_, err := bot.Send(tgbotapi.NewMessage(chatID, "Great! Now enter the text:"))
				if err != nil {
					log.Fatal("Error sending message: ", err)
				}

				userStates[chatID] = waitingTextMessage

			case "waiting_for_text":
				promt := Promt{Description: userInputs[chatID].Description, Text: msgText}
				userInputs[chatID] = promt
				go func(promt Promt, chatID int64) {
					mp3FileName, err := SendPromt(&promt)
					if err != nil {
						log.Printf("Error sending prompt: %v\n", err)
						_, err := bot.Send(tgbotapi.NewMessage(chatID, "An error occurred while processing the request. Try again later."))
						if err != nil {
							log.Fatal("Error sending message: ", err)
						}
					} else {
						file := tgbotapi.NewDocumentUpload(chatID, mp3FileName)
						_, err := bot.Send(file)
						if err != nil {
							log.Printf("Error sending file: %v\n", err)
						}
					}
					delete(userStates, chatID)
					delete(userInputs, chatID)
				}(promt, chatID)
			default:
				_, err := bot.Send(tgbotapi.NewMessage(chatID, "Unknown command. Use /start."))
				if err != nil {
					log.Fatal("Error sending message: ", err)
				}

			}
		}
	}()

	// A goroutine to get updates from the bot.
	go func() {
		for update := range updates {
			messageChan <- update
		}
	}()

	select {}
}
