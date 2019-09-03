package twitter

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/url"

	"github.com/ChimeraCoder/anaconda"
	"github.com/mpppk/sutaba-server/pkg/domain/model"
)

func CreateCRCToken(crcToken, consumerSecret string) string {
	mac := hmac.New(sha256.New, []byte(consumerSecret))
	mac.Write([]byte(crcToken))
	return "sha256=" + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func PostReply(api *anaconda.TwitterApi, text, toTweetIDStr string) (*model.Tweet, error) {
	v := url.Values{}
	v.Set("in_reply_to_status_id", toTweetIDStr)
	anacondaTweet, err := api.PostTweet(text, v)
	return ToTweet(&anacondaTweet), err
}