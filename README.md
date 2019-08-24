# Telepathy

An universal messenger botting platform

[![pipeline status](https://gitlab.com/kavenc/telepathy/badges/master/pipeline.svg)](https://gitlab.com/kavenc/telepathy/commits/master)

## Overview

Telepathy implements an abstraction layer of messenger botting APIs.
The idea is to decouple messenger APIs with bot logic.
With the provided abstraction layer, bot developers are possible to:

1. Focus on bot logic without struggling with various messenger APIs

2. Develope once, and the bot is enabled on all supported messenger automatically

3. Develop functionalities which involve cross-messenger communications (e.g. messesage forwarding between Slack and Discord)

## Supported Messengers

|Messenger|Go Binding|
|---------|----------|
|Discord|<https://github.com/bwmarrin/discordgo>|
|Slack|<https://github.com/nlopes/slack>|
|LINE|<https://github.com/line/line-bot-sdk-go>|

## Implemented Bot Features

### Message Forwarding

N-to-N message forwarding between any messenger any channels.

![image](https://i.imgur.com/yMr9tna.gif)

### Twitch Channel Online Notification

Prompt a notification to messenger channel when the specified stream has started/ended.

## Demo

To try out the demo implementation, add following bot users to your messenger/channel and send `teru help` for a list of available commands.

- **NOTICE/DISCLAIMER: Proceed at your own risk!**

  - Telepathy does not store any conversation. But the image will be uploaded to <https://imgur.com> when forwarding message with images.

  - Telepathy is not yet (actually far from) a mature application. All features should not be considered as reliable. We shall not be liable for any consequences resulting from using the bots below.

|Messenger|Bot Link/QR code|
|---------|----------|
|Discord|[Invite Telepathy](https://discordapp.com/api/oauth2/authorize?client_id=470906393470435329&permissions=100352&scope=bot)|
|Slack|<a href="https://slack.com/oauth/authorize?client_id=182680681824.654237744439&scope=bot"><img alt="Add to Slack" height="40" width="139" src="https://platform.slack-edge.com/img/add_to_slack.png" srcset="https://platform.slack-edge.com/img/add_to_slack.png 1x, https://platform.slack-edge.com/img/add_to_slack@2x.png 2x"></a>|
|LINE|<a href="https://line.me/R/ti/p/%40eso9171h"><img height="36" border="0" alt="加入好友" src="https://scdn.line-apps.com/n/line_add_friends/btn/zh-Hant.png"></a>|

## Hosting

Telepathy is now staging on Heroku. The provided `Procfile` and `Gopkg.toml` make it possible to be deployed by git push. Telepathy also depending on:

- MongoDB: MongoDB Atlas Free plan usaully works.

- Imgur: For handling image messages.

The following environment variables are necessary:

|Variable Name|Comment|
|-------------|-------|
|DISCORD_BOT_TOKEN|Discord Bot token|
|IMGUR_CLIENT_ID|Imgur API client ID|
|LINE_CHANNEL_SECRET|LINE API secret|
|LINE_CHANNEL_TOKEN|LINE API token|
|MONGODB_NAME|The database name of MongoDB|
|MONGODB_URL|The MongoDB server url, including account and password. (e.g. `mongodb+srv://(username):(authe token)@(database id).mongodb.net/test?retryWrites=true`)|
|SLACK_BOT_TOKEN|Slack app bot token|
|SLACK_CLIENT_ID|Slack app client ID (needed for OAuth)|
|SLACK_CLIENT_SECRET|Slack app client secret (needed for OAuth)|
|SLACK_SIGNING_SECRET|Slack message sign secret. (validate Slack reqests)|
|TWITCH_CLIENT_ID|Twitch api client ID|
|TWITCH_SECRET|Twitch secret for validating webusub notifications|

Note that variables for `IMGUR_*` and `MONGODB_*` are needed for Telepathy core.
The others can be optional and only needed if you enabled the service/messenger.
Service and messenger suport can be disabled by removing importing and configuring lines in `cmd/telepathy/telepathy.go`
