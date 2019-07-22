package telepathy

// MsgrUserProfile holds the information of a messenger user
type MsgrUserProfile struct {
	ID          string
	DisplayName string
}

// InboundMessage models a message send to Telepthy bot
type InboundMessage struct {
	FromChannel     Channel
	SourceProfile   *MsgrUserProfile
	Text            string
	IsDirectMessage bool
	Image           *Image
}

// OutboundMessage models a message send to user (through messenger)
type OutboundMessage struct {
	ToChannel Channel
	AsName    string // Sent the message as the specified user name
	Text      string // Message content
	Image     *Image // Image to be sent along with the message
}

// Reply constructs an OutboundMessage targeting to the channel where the InboundMessage came from
func (im InboundMessage) Reply() OutboundMessage {
	return OutboundMessage{
		ToChannel: im.FromChannel,
	}
}
