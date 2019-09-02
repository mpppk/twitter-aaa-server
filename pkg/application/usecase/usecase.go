package usecase

import (
	"fmt"
	"strings"
	"time"

	"github.com/mpppk/sutaba-server/pkg/application/service"

	"github.com/mpppk/sutaba-server/pkg/application/output"

	"github.com/mpppk/sutaba-server/pkg/application/repository"

	"github.com/mpppk/sutaba-server/pkg/domain/model"

	"github.com/mpppk/sutaba-server/pkg/util"

	"golang.org/x/xerrors"
)

type PostPredictTweetUseCaseConfig struct {
	SendUser             model.TwitterUser
	TargetKeyword        string
	ErrorTweetMessage    string
	SorryTweetMessage    string
	TwitterPresenter     output.MessagePresenter
	ClassifierRepository repository.ImageClassifierRepository
	MessageConverter     output.MessageConverter
}

type PostPredictTweetUseCase struct {
	conf             *PostPredictTweetUseCaseConfig
	twitterPresenter output.MessagePresenter
	messageConverter output.MessageConverter
}

func NewPostPredictTweetUsecase(conf *PostPredictTweetUseCaseConfig) *PostPredictTweetUseCase {
	return &PostPredictTweetUseCase{
		conf:             conf,
		twitterPresenter: conf.TwitterPresenter,
		messageConverter: conf.MessageConverter,
	}
}

func (p *PostPredictTweetUseCase) isTargetTweet(tweet *model.Tweet) (bool, string) {
	if len(tweet.MediaURLs) == 0 {
		return false, "tweet is ignored because it has no media"
	}
	if !strings.Contains(tweet.Text, p.conf.TargetKeyword) {
		return false, "tweet is ignored because it has no keyword"
	}

	if tweet.User.ID == p.conf.SendUser.ID {
		return false, "tweet is ignored because it is sent by bot"
	}
	return true, ""
}

func (p *PostPredictTweetUseCase) ReplyToUser(tweet *model.Tweet) (string, error) {
	ok, reason := p.isTargetTweet(tweet)
	if ok {
		f := func() error {
			tweetText, err := p.tweetToPredText(tweet)
			if err != nil {
				return err
			}
			return p.twitterPresenter.ReplyWithQuote(
				tweet.User,
				tweet.GetIDStr(),
				tweet.GetIDStr(),
				tweet.User.ScreenName,
				tweetText,
			)
		}
		err := f()
		if err != nil {
			errTweetText := p.conf.ErrorTweetMessage + fmt.Sprintf(" %v", time.Now())

			if err := p.twitterPresenter.Post(p.conf.SendUser, errTweetText); err != nil {
				util.LogPrintlnInOneLine("failed to tweet error notify message", err)
			}

			if err := p.twitterPresenter.Post(p.conf.SendUser, p.conf.SorryTweetMessage); err != nil {
				util.LogPrintlnInOneLine("failed to tweet error notify message", err)
			}
			return "", xerrors.Errorf("error occurred in ReplyToUser: %w", err)
		}
		return "", nil
	}

	if !tweet.HasQuoteTweet() {
		return reason, nil
	}

	// Check quote tweet
	ok, quoteReason := p.isTargetTweet(tweet.QuoteTweet)
	if !ok {
		return reason + ", and " + quoteReason, nil
	}
	f := func() error {
		tweetText, err := p.tweetToPredText(tweet.QuoteTweet)
		if err != nil {
			return err
		}

		err = p.twitterPresenter.ReplyWithQuote(
			tweet.User,
			tweet.GetIDStr(),
			tweet.QuoteTweet.GetIDStr(),
			tweet.QuoteTweet.User.ScreenName,
			tweetText,
		)
		if err != nil {
			return xerrors.Errorf("failed to post tweet: %v", err)
		}

		return nil
	}
	err := f()
	if err != nil {
		errTweetText := p.conf.ErrorTweetMessage + fmt.Sprintf(" %v", time.Now())
		if err := p.twitterPresenter.Post(p.conf.SendUser, errTweetText); err != nil {
			util.LogPrintlnInOneLine("failed to tweet error notify message", err)
		}

		if err := p.twitterPresenter.Post(p.conf.SendUser, p.conf.SorryTweetMessage); err != nil {
			util.LogPrintlnInOneLine("failed to tweet error notify message", err)
		}
		return "", xerrors.Errorf("error occurred in JudgeAndPostPredictTweetUseCase: %w", err)
	}
	return "", nil
}

func (p *PostPredictTweetUseCase) tweetToPredText(tweet *model.Tweet) (string, error) {
	mediaBytes, err := service.DownloadMediaFromTweet(tweet, 3, 1)
	if err != nil {
		return "", err
	}

	classifyResult, err := p.conf.ClassifierRepository.Do(mediaBytes)
	if err != nil {
		return "", xerrors.Errorf("failed to classifyResult: %v", err)
	}

	tweetText, err := p.messageConverter.GenerateResultMessage(classifyResult)
	if err != nil {
		return "", xerrors.Errorf("failed to convert classifyResult result to tweet text: %v", err)
	}
	return tweetText, err
}
