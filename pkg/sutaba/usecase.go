package sutaba

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/mpppk/sutaba-server/pkg/classifier"

	"github.com/ChimeraCoder/anaconda"
	"github.com/mpppk/sutaba-server/pkg/twitter"
)

type PostPredictTweetUseCaseConfig struct {
	SendUser          *twitter.User
	ClassifierClient  *classifier.Classifier
	ErrorTweetMessage string
	SorryTweetMessage string
}

type PostPredictTweetUseCase struct {
	conf *PostPredictTweetUseCaseConfig
}

func NewPostPredictTweetUsecase(conf *PostPredictTweetUseCaseConfig) *PostPredictTweetUseCase {
	return &PostPredictTweetUseCase{
		conf: conf,
	}
}

func (p *PostPredictTweetUseCase) isTargetTweet(tweet *anaconda.Tweet) (bool, string) {
	entityMediaList := tweet.Entities.Media
	if entityMediaList == nil || len(entityMediaList) == 0 {
		return false, "tweet is ignored because it has no media"
	}

	if !strings.Contains(tweet.Text, p.conf.SendUser.TargetKeyword) {
		return false, "tweet is ignored because it has no keyword"
	}

	if tweet.User.Id == p.conf.SendUser.ID {
		return false, "tweet is ignored because it is sent by bot"
	}
	return true, ""
}

func (p *PostPredictTweetUseCase) ReplyToUsers(tweet *anaconda.Tweet, subscribeUsers []*twitter.User) ([]*anaconda.Tweet, []string, error) {
	var postedTweets []*anaconda.Tweet
	var ignoreReasons []string
	for _, subscribeUser := range subscribeUsers {
		postedTweet, ignoredReason, err := p.ReplyToUser(tweet, subscribeUser)
		if err != nil {
			return nil, nil, err
		}
		if ignoredReason == "" {
			postedTweets = append(postedTweets, postedTweet)
			continue
		}
		ignoreReasons = append(ignoreReasons, ignoredReason)
	}
	return postedTweets, ignoreReasons, nil
}

func (p *PostPredictTweetUseCase) ReplyToUser(tweet *anaconda.Tweet, subscribeUser *twitter.User) (*anaconda.Tweet, string, error) {
	if tweet.InReplyToUserID != subscribeUser.ID {
		return nil, "tweet is ignored because it is not sent to subscribe user", nil
	}
	ok, reason := p.isTargetTweet(tweet)
	if ok {
		postedTweet, err := p.postPredictTweet(tweet, "")
		if err != nil {
			errTweetText := p.conf.ErrorTweetMessage + fmt.Sprintf(" %v", time.Now())
			if subscribeUser.IsErrorReporter {
				subscribeUser.PostErrorTweet(errTweetText, p.conf.SorryTweetMessage, tweet.IdStr, tweet.User.ScreenName)
			}
			return nil, "", xerrors.Errorf("error occurred in JudgeAndPostPredictTweetUseCase: %w", err)
		}
		return postedTweet, "", nil
	}

	if tweet.QuotedStatus == nil {
		return nil, reason, nil
	}

	// Check quote tweet
	ok, quoteReason := p.isTargetTweet(tweet.QuotedStatus)
	if !ok {
		return nil, reason + ", and " + quoteReason, nil
	}
	f := func() (*anaconda.Tweet, error) {
		tweetText, err := p.tweetToPredText(tweet.QuotedStatus)
		if err != nil {
			return nil, err
		}
		postedTweet, err := p.conf.SendUser.PostReplyWithQuote(tweetText, tweet.QuotedStatus, tweet.IdStr, []string{tweet.User.ScreenName})
		if err != nil {
			return nil, xerrors.Errorf("failed to post tweet: %v", err)
		}
		return &postedTweet, nil
	}
	postedTweet, err := f()
	if err != nil {
		errTweetText := p.conf.ErrorTweetMessage + fmt.Sprintf(" %v", time.Now())
		if subscribeUser.IsErrorReporter {
			subscribeUser.PostErrorTweet(errTweetText, p.conf.SorryTweetMessage, tweet.IdStr, tweet.User.ScreenName)
		}
		return nil, "", xerrors.Errorf("error occurred in JudgeAndPostPredictTweetUseCase: %w", err)
	}
	return postedTweet, "", nil
}

func (p *PostPredictTweetUseCase) postPredictTweet(tweet *anaconda.Tweet, tweetTextPrefix string) (*anaconda.Tweet, error) {
	tweetText, err := p.tweetToPredText(tweet)
	if err != nil {
		return nil, err
	}
	postedTweet, err := p.conf.SendUser.PostByTweetType(tweetTextPrefix+tweetText, tweet)
	if err != nil {
		return nil, xerrors.Errorf("failed to post tweet: %v", err)
	}
	return &postedTweet, nil
}

func (p *PostPredictTweetUseCase) tweetToPredText(tweet *anaconda.Tweet) (string, error) {
	mediaBytes, err := twitter.DownloadEntityMediaFromTweet(tweet, 3, 1)
	if err != nil {
		return "", err
	}

	predict, err := p.conf.ClassifierClient.Predict(mediaBytes)
	if err != nil {
		return "", xerrors.Errorf("failed to predict: %v", err)
	}

	tweetText, err := PredToText(predict)
	if err != nil {
		return "", xerrors.Errorf("failed to convert predict result to tweet text: %v", err)
	}
	return tweetText, err
}