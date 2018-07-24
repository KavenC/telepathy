package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/line/line-bot-sdk-go/linebot"
)

func main() {
	line, err := linebot.New(
		os.Getenv("LINE_CHANNEL_SECRET"),
		os.Getenv("LINE_CHANNEL_TOKEN"),
	)
	if err != nil {
		log.Fatal("LineBot Init failed: " + err.Error())
	}
	discord, err := discordgo.New("Bot " + os.Getenv("DISCORD_BOT_TOKEN"))
	if err != nil {
		log.Fatal("DiscordGo Init failed: " + err.Error())
	}

	// Start delivery
	discordChannel := ""
	lineChannel := ""

	// Register the messageCreate func as a callback for MessageCreate events.
	discord.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {

		// Ignore all messages created by the bot itself
		// This isn't required in this specific example but it's a good practice.
		if m.Author.ID == s.State.User.ID {
			return
		}

		for _, mentioned := range m.Mentions {
			if mentioned.ID == s.State.User.ID {
				log.Print("Bot is mentioned")
				if strings.Contains(m.Content, "start") {
					s.ChannelMessageSend(m.ChannelID, "Message Forwarding is started in this Channel.")
					discordChannel = m.ChannelID
					return
				}
			}
		}

		// Forward message to line
		if lineChannel != "" {
			message := linebot.NewTextMessage(m.Author.Username + ": " + m.Content)
			_, err = line.PushMessage(lineChannel, message).Do()
			if err != nil {
				log.Fatal(err)
			}
		}
	})

	// Open a websocket connection to Discord and begin listening.
	err = discord.Open()
	if err != nil {
		fmt.Println("error opening connection,", err)
		return
	}

	// Setup HTTP Server for receiving requests from LINE platform
	http.HandleFunc("/callback", func(w http.ResponseWriter, req *http.Request) {
		events, err := line.ParseRequest(req)
		if err != nil {
			if err == linebot.ErrInvalidSignature {
				w.WriteHeader(400)
			} else {
				w.WriteHeader(500)
			}
			return
		}

		lineCmdPrefix := "!delivery "
		for _, event := range events {
			if event.Type == linebot.EventTypeMessage {
				switch message := event.Message.(type) {
				case *linebot.TextMessage:
					// Parse Command
					if strings.HasPrefix(message.Text, lineCmdPrefix) {
						cmd := strings.TrimSpace(strings.TrimPrefix(message.Text, lineCmdPrefix))
						if cmd == "start" {
							if event.Source.GroupID != "" {
								lineChannel = event.Source.GroupID
							} else if event.Source.UserID != "" {
								lineChannel = event.Source.UserID
							} else if event.Source.RoomID != "" {
								lineChannel = event.Source.RoomID
							}
							message := linebot.NewTextMessage("Message forwarding is started in this Channel")
							_, err = line.PushMessage(lineChannel, message).Do()
							if err != nil {
								log.Fatal(err)
							}
						}
					} else if discordChannel != "" {
						// Forward Message
						var sender *linebot.UserProfileResponse
						if event.Source.GroupID != "" {
							senderCall := line.GetGroupMemberProfile(event.Source.GroupID, event.Source.UserID)
							sender, _ = senderCall.Do()
						} else if event.Source.RoomID != "" {
							senderCall := line.GetRoomMemberProfile(event.Source.RoomID, event.Source.UserID)
							sender, _ = senderCall.Do()
						} else {
							senderCall := line.GetProfile(event.Source.UserID)
							sender, _ = senderCall.Do()
						}
						discord.ChannelMessageSend(discordChannel, sender.DisplayName+": "+message.Text)
					}
				}
			}
		}
	})

	// This is just sample code.
	// For actual use, you must support HTTPS by using `ListenAndServeTLS`, a reverse proxy or something else.
	if err := http.ListenAndServe(":"+os.Getenv("PORT"), nil); err != nil {
		log.Fatal(err)
	}

	discord.Close()
}
