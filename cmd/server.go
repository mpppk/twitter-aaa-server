package cmd

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"

	"github.com/mpppk/sutaba-server/pkg/twitter"

	"github.com/labstack/echo/v4/middleware"

	"github.com/ChimeraCoder/anaconda"

	"github.com/mpppk/sutaba-server/internal/option"

	"github.com/labstack/echo/v4"
	"github.com/spf13/afero"

	"github.com/spf13/cobra"
)

type CRCRequest struct {
	CRCToken string `json:"crc_token" query:"crc_token"`
}

type CRCResponse struct {
	ResponseToken string `json:"response_token"`
}

type ImagePredictResponse struct {
	Pred       string `json:"pred"`
	Confidence string `json:"confidence"`
}

type TweetCreateEvents struct {
	ForUserId         string           `json:"for_user_id"`
	TweetCreateEvents []anaconda.Tweet `json:"tweet_create_events"`
}

func bodyDumpHandler(c echo.Context, reqBody, resBody []byte) {
	fmt.Printf("Request Body: %v\n", string(reqBody))
	fmt.Printf("Response Body: %v\n", string(resBody))
}

func newServerCmd(fs afero.Fs) (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Start server",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			conf, err := option.NewServerCmdConfigFromViper()
			if err != nil {
				return err
			}

			e := echo.New()
			e.Use(middleware.BodyDump(bodyDumpHandler))

			endpoint := "/twitter/aaa"
			e.GET(endpoint, func(c echo.Context) error {
				req := new(CRCRequest)
				if err = c.Bind(req); err != nil {
					return err
				}
				response := &CRCResponse{ResponseToken: CreateCRCToken(req.CRCToken, conf.TwitterConsumerSecret)}
				return c.JSON(http.StatusOK, response)
			})

			e.POST(endpoint, func(c echo.Context) error {
				events := new(TweetCreateEvents)
				if err = c.Bind(events); err != nil {
					return err
				}
				fmt.Printf("%#v\n", events)
				if events.TweetCreateEvents == nil {
					return c.NoContent(http.StatusNoContent)
				}

				tweets := events.TweetCreateEvents
				if len(tweets) == 0 {
					return c.NoContent(http.StatusNoContent)
				}

				tweet := tweets[0]
				entityMediaList := tweet.Entities.Media
				if entityMediaList == nil || len(entityMediaList) == 0 {
					return c.NoContent(http.StatusNoContent)
				}

				entityMedia := entityMediaList[0]
				mediaBytes, err := twitter.DownloadEntityMedia(&entityMedia)
				if err != nil {
					log.Println(err)
					return c.String(http.StatusInternalServerError, fmt.Sprintf("failed to download media: %s", err))
				}
				mediaBuffer := bytes.NewBuffer(mediaBytes)
				// リクエストボディのデータを受け取るio.Writerを生成する。
				body := &bytes.Buffer{}

				// データのmultipartエンコーディングを管理するmultipart.Writerを生成する。
				// ランダムなbase-16バウンダリが生成される。
				mw := multipart.NewWriter(body)

				// ファイルに使うパートを生成する。
				// ヘッダ以外はデータは書き込まれない。
				// fieldnameとfilenameの値がヘッダに含められる。
				// ファイルデータを書き込むio.Writerが返却される。
				fw, err := mw.CreateFormFile("file", "image")

				// fwで作ったパートにファイルのデータを書き込む
				if _, err = io.Copy(fw, mediaBuffer); err != nil {
					log.Println(err)
					return c.String(http.StatusInternalServerError, fmt.Sprintf("%s", err))
				}

				// リクエストのContent-Typeヘッダに使う値を取得する（バウンダリを含む）
				contentType := mw.FormDataContentType()

				// 書き込みが終わったので最終のバウンダリを入れる
				if err = mw.Close(); err != nil {
					log.Println(err)
					return c.String(http.StatusInternalServerError, fmt.Sprintf("%s", err))
				}

				// contentTypeとbodyを使ってリクエストを送信する
				url := "https://sutaba-lkui2qyzba-an.a.run.app/predict"
				resp, err := http.Post(url, contentType, body)
				if err != nil {
					log.Println(err)
					return c.String(http.StatusInternalServerError, fmt.Sprintf("%s", err))
				}

				var predictResponse ImagePredictResponse
				if err := json.NewDecoder(resp.Body).Decode(&predictResponse); err != nil {
					log.Println(err)
					return c.String(http.StatusInternalServerError, fmt.Sprintf("%s", err))
				}

				log.Printf("predict: %#v\n", predictResponse)

				if err = resp.Body.Close(); err != nil {
					log.Println(err)
					return c.String(http.StatusInternalServerError, fmt.Sprintf("%s", err))
				}
				return c.NoContent(http.StatusNoContent)
			})

			port := "1323"
			envPort := os.Getenv("PORT")
			if envPort != "" {
				port = envPort
			}
			e.Logger.Fatal(e.Start(":" + port))
			return nil
		},
	}
	return cmd, nil
}

func CreateCRCToken(crcToken, consumerSecret string) string {
	mac := hmac.New(sha256.New, []byte(consumerSecret))
	mac.Write([]byte(crcToken))
	return "sha256=" + base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func init() {
	cmdGenerators = append(cmdGenerators, newServerCmd)
}
