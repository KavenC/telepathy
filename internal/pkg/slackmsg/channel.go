package slackmsg

import "encoding/json"

type unqiueChannel struct {
	TeamID    string
	ChannelID string
}

func newUniqueChannel(encoded string) (*unqiueChannel, error) {
	ch := unqiueChannel{}
	err := json.Unmarshal([]byte(encoded), &ch)
	return &ch, err
}

func (c unqiueChannel) encode() (string, error) {
	enc, err := json.Marshal(c)
	return string(enc), err
}
