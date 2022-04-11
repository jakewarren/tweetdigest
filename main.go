//nolint:funlen,gosec
package main

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ChimeraCoder/anaconda"
	"github.com/jakewarren/metascraper"
	apppaths "github.com/muesli/go-app-paths"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gopkg.in/gomail.v2"
)

type app struct {
	Client *anaconda.TwitterApi
	Config struct {
		Threshold       time.Duration
		ConfigFile      string
		TweetCount      int
		Verbose         bool
		IncludeRetweets bool
		IncludeReplies  bool
	}
}

type SeverityHook struct{}

// hasErrorOccurred will be set to 1 if an error has occurred throughout the execution
// this error is then reported via the exit code so that cronic will fire an email.
var hasErrorOccured int

// Run hooks into error events to record that an error has occurred
func (h SeverityHook) Run(e *zerolog.Event, level zerolog.Level, msg string) {
	if level >= zerolog.ErrorLevel {
		hasErrorOccured = 1
		e.Caller()
	}
}

var (
	// build information set by ldflags
	appName   = "tweetdigest"
	version   = "(ﾉ☉ヮ⚆)ﾉ ⌒*:･ﾟ✧"
	commit    = "(ﾉ☉ヮ⚆)ﾉ ⌒*:･ﾟ✧"
	buildDate = "(ﾉ☉ヮ⚆)ﾉ ⌒*:･ﾟ✧"

	// simple regex to match on URLs
	urlRE = regexp.MustCompile(`(?m)https?:\/\/[^\s]+`)
)

func main() {
	a := app{}

	pflag.Usage = func() {
		fmt.Printf("Description: %s\n\n", "compiles tweets into an email digest")
		fmt.Printf("Usage: %s -d [duration] [twitter username]\n\n", os.Args[0])
		fmt.Printf("Options:\n")
		pflag.PrintDefaults()
		os.Exit(0)
	}

	showVersion := pflag.BoolP("version", "V", false, "show version information")
	pflag.IntVar(&a.Config.TweetCount, "tweet-count", 50, "number of tweets to analyze (max 200)")
	pflag.StringVarP(&a.Config.ConfigFile, "config", "c", "", "filepath to the config file")
	pflag.DurationVarP(&a.Config.Threshold, "duration", "d", 0, "how far back to include tweets in the digest (example: \"-24h\")")
	pflag.StringSliceP("email-to", "t", nil, "email address(es) to send the report to")
	pflag.BoolVar(&a.Config.IncludeRetweets, "include-retweets", true, "include retweets in the digest")
	pflag.BoolVar(&a.Config.IncludeReplies, "include-replies", true, "include replies in the digest")
	pflag.BoolVarP(&a.Config.Verbose, "verbose", "v", false, "enable verbose output")
	pflag.Parse()
	_ = viper.BindPFlags(pflag.CommandLine)

	log.Logger = zerolog.New(os.Stdout).Hook(SeverityHook{}).With().Caller().Timestamp().Logger()
	if a.Config.Verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.ErrorLevel)
	}

	if *showVersion {
		fmt.Printf(`%s:
    version     : %s
    git hash    : %s
    build date  : %s
    go version  : %s
    go compiler : %s
    platform    : %s/%s
`, appName, version, commit, buildDate, runtime.Version(), runtime.Compiler, runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	// check for required args
	if pflag.NArg() < 1 {
		log.Fatal().Msg("twitter screenname was not provided")
	}
	if a.Config.Threshold == 0 {
		log.Fatal().Msg("threshold duration was not provided")
	}

	// load up config
	if a.Config.ConfigFile != "" {
		viper.SetConfigFile(a.Config.ConfigFile)
	} else {
		viper.SetConfigName("tweetdigest") // name of config file (without extension)
		viper.AddConfigPath(".")           // optionally look for config in the working directory
		viper.AddConfigPath("$HOME")       // optionally look for config in home directory

		userScope := apppaths.NewScope(apppaths.User, "", "tweetdigest")
		xdgConfigDir, configDirErr := userScope.ConfigPath("")
		if configDirErr == nil {
			viper.AddConfigPath(xdgConfigDir)
		}
	}

	if err := viper.ReadInConfig(); err != nil { // Handle errors reading the config file
		log.Fatal().Err(err).Msg("Fatal error config file")
	}

	// init Twitter API
	anaconda.SetConsumerKey(viper.GetString("consumer_key"))
	anaconda.SetConsumerSecret(viper.GetString("consumer_secret"))
	a.Client = anaconda.NewTwitterApi(viper.GetString("access_token"), viper.GetString("access_token_secret"))

	tweets := make([]anaconda.Tweet, 0)
	for _, u := range pflag.Args() {
		t := a.getTweetsForUser(u)
		tweets = append(tweets, t...)
	}

	if len(tweets) == 0 {
		return
	}

	m := gomail.NewMessage()
	m.SetAddressHeader("From", viper.GetString("email_from.address"), viper.GetString("email_from.name"))
	m.SetHeader("To", viper.GetStringSlice("email-to")...)
	m.SetHeader("Subject", fmt.Sprintf("@%s Tweet Digest for %s", strings.Join(pflag.Args(), "/@"), time.Now().Format("1/2/06")))
	m.SetBody("text/html", a.generateHTML(tweets))
	d := gomail.Dialer{
		Host:     viper.GetString("email_server.server"),
		Port:     viper.GetInt("email_server.port"),
		Username: viper.GetString("email_server.username"),
		Password: viper.GetString("email_server.password"),
	}
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}
	if mailErr := d.DialAndSend(m); mailErr != nil {
		panic(mailErr)
	}

	os.Exit(hasErrorOccured)
}

func (a app) getTweetsForUser(s string) []anaconda.Tweet {
	v := url.Values{}
	v.Set("screen_name", s)
	v.Set("count", strconv.Itoa(a.Config.TweetCount))

	var timeline []anaconda.Tweet

	// for some reason Twitter will occasionally only return one tweet, so use this hacky retry method
	for i := 1; i <= 5; i++ {
		var err error
		timeline, err = a.Client.GetUserTimeline(v)
		if err != nil {
			log.Error().Err(err).Msg("error getting timeline")
		}

		tweetCount := len(timeline)
		log.Debug().Int("tweet-count", tweetCount).Int("attempt", i).Msg("pulled down tweets")
		if tweetCount > 1 || a.Config.TweetCount == 1 {
			break
		}

		backoffInterval := 5
		log.Debug().Msgf("retrying after %d seconds...", i*backoffInterval)
		time.Sleep(time.Duration(i*backoffInterval) * time.Second)
	}

	dateThreshold := time.Now().Local().Add(a.Config.Threshold)

	tweets := make([]anaconda.Tweet, 0)

	for _, tweet := range timeline {
		cTime, _ := tweet.CreatedAtTime()
		cTime = cTime.Local() // convert to local timezone

		if cTime.After(dateThreshold) {

			if !a.Config.IncludeRetweets && tweet.RetweetedStatus != nil {
				continue
			}

			if !a.Config.IncludeReplies && tweet.InReplyToStatusIdStr != "" && (tweet.InReplyToUserID != tweet.User.Id) {
				continue
			}

			tweets = append([]anaconda.Tweet{tweet}, tweets...)
		}
	}

	return tweets
}

type emailBody struct {
	Tweets []anaconda.Tweet
}

func (a app) generateHTML(tweets []anaconda.Tweet) string {
	var (
		e   emailBody
		err error
	)
	e.Tweets = tweets

	funcMap := template.FuncMap{
		"formatTime": func(t anaconda.Tweet) template.HTML {
			// get the creation time and convert to user's local timezone
			cTime, _ := t.CreatedAtTime()
			cTime = cTime.Local()
			return template.HTML(cTime.Format("Jan 2"))
		},
		// enrich URLs by using the twitter images from URLs if they provide the supported metadata
		"getTwitterImage": func(url string) template.HTML {
			var output string

			log.Debug().Str("url", url).Msg("fetching image for URL")

			if strings.HasPrefix(url, "https://twitter.com/") && strings.Contains(url, "/status/") {
				tweetCardText, tweetURL := a.generateTwitterCard(url)
				output += tweetCardText
				if tweetURL != "" {
					url = tweetURL
				}
			}

			p, metaErr := metascraper.Scrape(url)
			if metaErr != nil {
				log.Error().Str("url", url).Err(metaErr).Msg("error getting metadata for an url")
				return ""
			}

			for _, m := range p.MetaData() {
				if m.Name == "twitter:image" || m.Name == "og:image" || m.Name == "twitter:image:src" {
					output += fmt.Sprintf(`<img src="%s" style="max-width:100%%; padding-bottom:5px">`, m.Content)
				}
			}

			return template.HTML(output)
		},
		// unshorten a single URL
		"unshortenURL": func(url string) template.HTML {
			finalURL, _ := unshortenURL(url)
			return template.HTML(finalURL)
		},
		// use regex to attempt to unshorten all URLs in a block of text
		"unshortenURLsinText": func(text string) template.HTML {
			output := text

			for _, match := range urlRE.FindAllString(text, -1) {
				finalURL, unshortenErr := unshortenURL(match)
				if unshortenErr == nil {
					output = strings.ReplaceAll(output, match, finalURL)
				}
			}

			return template.HTML(output)
		},
	}

	t := template.New("emailTmpl").Funcs(funcMap)
	if t, err = t.Parse(emailTemplate); err != nil {
		panic(err)
	}
	var buf bytes.Buffer
	if err = t.Execute(&buf, e); err != nil {
		log.Error().Err(err).Msg("error executing template")
	}

	return buf.String()
}

// TODO: make this prettier
func (a app) generateTwitterCard(tweetURL string) (string, string) {
	var output string

	log.Debug().Str("url", tweetURL).Msg("generating twitter card")

	v := url.Values{}
	v.Set("url", tweetURL)
	v.Set("dnt", "true")

	o, err := a.Client.GetOEmbed(v)
	if err == nil {
		output = o.Html
	} else {
		log.Error().Err(err).Str("url", tweetURL).Msg("error generating tweet card")
	}

	return output, tweetURL
}

func unshortenURL(url string) (string, error) {
	var output string

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
	}
	client := http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
	}

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible;)")

	resp, unshortenErr := client.Do(req)
	if unshortenErr == nil {
		output = resp.Request.URL.String()
	}
	resp.Body.Close()
	return output, unshortenErr
}

const emailTemplate = `
<html xmlns="http://www.w3.org/1999/xhtml"
    style='box-sizing:border-box; font-family:"Helvetica Neue", Helvetica, Arial, sans-serif'>

<head>
    <meta http-equiv="Content-Type" content="text/html; charset=utf-8=">
    <meta name="viewport" content="width=device-width">
    <title></title>


</head>

<body style="height:100%; margin:0; width:100%; background-color:#fff" height="100%" width="100%" bgcolor="#ffffff">

    <style type="text/css">
        @media only screen and (max-width: 640px) {
            body {
                padding: 0 !important
            }

            h1,
            h2,
            h3,
            h4 {
                font-weight: 800 !important;
                margin: 20px 0 5px !important
            }

            h1 {
                font-size: 22px !important
            }

            h2 {
                font-size: 18px !important
            }

            h3 {
                font-size: 16px !important
            }

            .container {
                padding: 0 !important;
                width: 100% !important
            }

            .content {
                padding: 0 !important
            }

            .content-wrap {
                padding: 10px !important
            }

            .invoice {
                width: 100% !important
            }
        }
    </style>
    <base target="_target">
    <table style="table-layout:fixed; width:100%; max-width:600px; clear:both !important; margin:0 auto !important"
        width="100%">

		{{range .Tweets}}
        

{{ if .RetweetedStatus }}

<tr>
            <td style="vertical-align:top; border:1px solid #E2E6E6; padding:5px; border-bottom:none" valign="top">
<img style="max-width:100%; display:inline; height:10px; padding-top:1px; vertical-align:baseline; width:auto" src="https://upload.wikimedia.org/wikipedia/commons/7/70/Retweet.png" height="10" valign="baseline" width="auto"> {{.User.ScreenName}} Retweeted <br>
                <table style="table-layout:fixed; width:100%" width="100%">
                    <tr>
						
                        <td style="vertical-align:top; text-align:center; width:60px" valign="top" align="center"
                            width="60">


                            <img style="max-width:100%; border-radius:50%; height:48px; min-width:48px; width:48px"
                                src="{{.RetweetedStatus.User.ProfileImageUrlHttps}}"
                                height="48" width="48">
                        </td>
                        <td style="vertical-align:top" valign="top">
                            <table cellpadding="0" cellspacing="0" border="0"
                                style="table-layout:fixed; width:100%; padding-left:5px" width="100%">
                                <tr>
                                    <td style="vertical-align:top" valign="top">

                                        <a href="https://twitter.com/{{.RetweetedStatus.User.ScreenName}}/status/{{.RetweetedStatus.Id}}"
                                            style="color:black; text-decoration:None">
                                            <strong>{{ .RetweetedStatus.User.Name }}</strong>
                                            <span>@{{.RetweetedStatus.User.ScreenName}}</span>
                                            <span style="float:right;">{{.RetweetedStatus | formatTime }}</span>
                                        </a>
                                    </td>
                                </tr>
                                <tr>
                                    <td style="vertical-align:top" valign="top">

                                        <p style="margin-bottom:10px; margin:0; padding-bottom:5px; white-space:pre-wrap">
{{ .RetweetedStatus.FullText | unshortenURLsinText}}    
										</p>

{{range .RetweetedStatus.ExtendedEntities.Media}}
<img src="{{.Media_url_https}}"  style="max-width:100%; padding-bottom:5px">
{{end}}

{{range .RetweetedStatus.Entities.Urls}}

<table style="table-layout:fixed; width:100%; border-radius:12px; border:1px solid #E2E6E6; padding:5px; margin:5px 0"   width="100%">
    <tr>

        <td style="vertical-align:top" valign="top">
            <table cellpadding="0" cellspacing="0" border="0"
                style="table-layout:fixed; width:100%; padding-left:5px"
                width="100%">
                <tr>
                    <td style="vertical-align:top" valign="top">
                        <p
                            style="margin-bottom:10px; margin:0; overflow:hidden; text-overflow:inherit; white-space:normal">
                            <a href="{{.Expanded_url | unshortenURL}}"
                                target="_blank"
                                style="color:#000; text-decoration:None">
                                {{.Expanded_url | getTwitterImage}}

                                <strong>{{.Expanded_url | unshortenURL}}</strong>
                            </a>
                        </p>
                    </td>
                </tr>

                

            </table>

        </td>
    </tr>
</table>

{{end}}

                                    </td>
                                </tr>


                                <td style="vertical-align:top" valign="top">
                                    <table style="table-layout:fixed; width:100%" width="100%">
                                        <tr>
                                            <a href="https://twitter.com/{{.User.ScreenName}}/status/{{.Id}}"
                                                target="_blank" style="color:#348eda; text-decoration:None">
                                                <p style="margin-bottom:10px; margin:0">

                                                    <span style="color:#4e555b; margin-right:28px">
                                                        <img src="https://upload.wikimedia.org/wikipedia/commons/7/70/Retweet.png"
                                                            style="max-width:100%; display:inline; height:16px; padding-top:1px; vertical-align:text-top; width:auto"
                                                            height="16" valign="text-top" width="auto">
                                                        <span>{{.RetweetedStatus.RetweetCount}}</span>
                                                    </span>
                                                    <span style="color:#4e555b; margin-right:28px">
                                                        <img src="https://upload.wikimedia.org/wikipedia/commons/c/c9/Twitter_favorite.png"
                                                            style="max-width:100%; display:inline; height:16px; padding-top:1px; vertical-align:text-top; width:auto"
                                                            height="16" valign="text-top" width="auto">
                                                        <span>{{.RetweetedStatus.FavoriteCount}}</span>
                                                    </span>
                                                </p>
                                            </a>
                                        </tr>
                                    </table>
                                </td>
                            </table>
                            <a href="https://twitter.com/{{.RetweetedStatus.User.ScreenName}}/"
                                target="_blank" style="color:#348eda; text-decoration:None"></a>
                        </td>
                    </tr>
                </table>
            </td>
        </tr>
{{ else }}
<tr>
            <td style="vertical-align:top; border:1px solid #E2E6E6; padding:5px; border-bottom:none" valign="top">

                <table style="table-layout:fixed; width:100%" width="100%">
                    <tr>

                        <td style="vertical-align:top; text-align:center; width:60px" valign="top" align="center"
                            width="60">
                            <img style="max-width:100%; border-radius:50%; height:48px; min-width:48px; width:48px"
                                src="{{.User.ProfileImageUrlHttps}}"
                                height="48" width="48">
                        </td>
                        <td style="vertical-align:top" valign="top">
                            <table cellpadding="0" cellspacing="0" border="0"
                                style="table-layout:fixed; width:100%; padding-left:5px" width="100%">
                                <tr>
                                    <td style="vertical-align:top" valign="top">

                                        <a href="https://twitter.com/{{.User.ScreenName}}/status/{{.Id}}"
                                            style="color:black; text-decoration:None">
                                            <strong>{{ .User.Name }}</strong>
                                            <span>@{{.User.ScreenName}}</span>
                                            <span style="float:right;">{{. | formatTime }}</span>
                                        </a>
                                    </td>
                                </tr>
                                <tr>
                                    <td style="vertical-align:top" valign="top">

                                        <p style="margin-bottom:10px; margin:0; padding-bottom:5px; white-space:pre-wrap">
{{ .FullText | unshortenURLsinText}}    
										</p>

{{range .ExtendedEntities.Media}}
<img src="{{.Media_url_https}}"  style="max-width:100%; padding-bottom:5px">
{{end}}

{{range .Entities.Urls}}

<table style="table-layout:fixed; width:100%; border-radius:12px; border:1px solid #E2E6E6; padding:5px; margin:5px 0"   width="100%">
    <tr>

        <td style="vertical-align:top" valign="top">
            <table cellpadding="0" cellspacing="0" border="0"
                style="table-layout:fixed; width:100%; padding-left:5px"
                width="100%">
                <tr>
                    <td style="vertical-align:top" valign="top">
                        <p
                            style="margin-bottom:10px; margin:0; overflow:hidden; text-overflow:inherit; white-space:normal">
                            <a href="{{.Expanded_url | unshortenURL}}"
                                target="_blank"
                                style="color:#000; text-decoration:None">
                                {{.Expanded_url | getTwitterImage}}

                                <strong>{{.Expanded_url | unshortenURL}}</strong>
                            </a>
                        </p>
                    </td>
                </tr>

                

            </table>
        </td>
    </tr>
</table>
{{end}}

                                    </td>
                                </tr>


                                <td style="vertical-align:top" valign="top">
                                    <table style="table-layout:fixed; width:100%" width="100%">
                                        <tr>
                                            <a href="https://twitter.com/{{.User.ScreenName}}/status/{{.Id}}"
                                                target="_blank" style="color:#348eda; text-decoration:None">
                                                <p style="margin-bottom:10px; margin:0">

                                                    <span style="color:#4e555b; margin-right:28px">
                                                        <img src="https://upload.wikimedia.org/wikipedia/commons/7/70/Retweet.png"
                                                            style="max-width:100%; display:inline; height:16px; padding-top:1px; vertical-align:text-top; width:auto"
                                                            height="16" valign="text-top" width="auto">
                                                        <span>{{.RetweetCount}}</span>
                                                    </span>
                                                    <span style="color:#4e555b; margin-right:28px">
                                                        <img src="https://upload.wikimedia.org/wikipedia/commons/c/c9/Twitter_favorite.png"
                                                            style="max-width:100%; display:inline; height:16px; padding-top:1px; vertical-align:text-top; width:auto"
                                                            height="16" valign="text-top" width="auto">
                                                        <span>{{.FavoriteCount}}</span>
                                                    </span>
                                                </p>
                                            </a>
                                        </tr>
                                    </table>
                                </td>
                            </table>
                            <a href="https://twitter.com/{{.User.ScreenName}}/"
                                target="_blank" style="color:#348eda; text-decoration:None"></a>
                        </td>
                    </tr>
                </table>
            </td>
        </tr>

{{ end }}
		{{end}}

        
    </table>






</body>

</html>
`
