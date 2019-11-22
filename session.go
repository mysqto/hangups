package hangups

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

var deviceName = oauth2.SetAuthURLParam("device_name", "hangups")

// Session represent a hangouts session
type Session struct {
	RefreshToken string
	Cookies      string
	Sapisid      string
}

var (
	scopes = []string{
		"https://www.google.com/accounts/OAuthLogin",
		"https://www.googleapis.com/auth/userinfo.email",
	} // only need scope for logging in

	oauthEndpoint = oauth2.Endpoint{
		AuthURL:  "https://accounts.google.com/o/oauth2/programmatic_auth", // interactive user login
		TokenURL: "https://accounts.google.com/o/oauth2/token",             // API endpoint to get access token from refresh token or auth code
	}
)

// Init a session
func (s *Session) Init() error {
	oauthConf := &oauth2.Config{
		ClientID:     "936475272427.apps.googleusercontent.com", // iOS id
		ClientSecret: "KWsJlkaMn1jGLxQpWxMnOox-",                // iOS secret
		Scopes:       scopes,
		Endpoint:     oauthEndpoint,
	}

	client, err := s.getOauthClient(oauthConf)
	if err != nil {
		return err
	}

	err = s.setCookies(client)
	if err != nil {
		return err
	}
	return nil
}

func (s *Session) setCookies(client *http.Client) error {
	cookieJar, _ := cookiejar.New(nil)
	client.Jar = cookieJar

	resp, err := client.Get("https://accounts.google.com/accounts/OAuthLogin?source=hangups&issueuberauth=1")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	uberauth, _ := ioutil.ReadAll(resp.Body)

	mergeSessionURL := fmt.Sprintf("https://accounts.google.com/MergeSession?service=mail&continue=http://www.google.com&uberauth=%s", uberauth)
	// url encode it
	url, _ := url.Parse(mergeSessionURL)
	q := url.Query()
	url.RawQuery = q.Encode()
	resp, err = client.Get(url.String())
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	u, _ := url.Parse("google.com")
	requiredCookies := map[string]string{
		"APISID":  "",
		"HSID":    "",
		"NID":     "",
		"SAPISID": "",
		"SID":     "",
		"SIDCC":   "",
		"SSID":    "",
	}
	cookies := make([]string, 0)
	for _, cookie := range client.Jar.Cookies(u) {
		_, found := requiredCookies[cookie.Name]
		if found {
			cookies = append(cookies, cookie.String())
		}
		if "SAPISID" == cookie.Name {
			s.Sapisid = cookie.Value
		}
	}

	s.Cookies = strings.Join(cookies, "; ")

	return nil
}

func (s *Session) getOauthClient(oauthConf *oauth2.Config) (*http.Client, error) {
	var oauthClient *http.Client
	var token *oauth2.Token
	var err error

	if s.RefreshToken == "" {
		token, err = tokenFromAuthCode(oauthConf)
	} else {
		token, err = tokenFromRefreshToken(oauthConf, s.RefreshToken)
	}

	if err != nil {
		return nil, err
	}

	s.RefreshToken = token.RefreshToken
	oauthClient = oauthConf.Client(context.TODO(), token)

	return oauthClient, nil
}

func tokenFromRefreshToken(oauthConf *oauth2.Config, refreshToken string) (*oauth2.Token, error) {
	// generate an expired token with the refreshToken and let TokenSource refresh it
	expiredToken := &oauth2.Token{RefreshToken: refreshToken, Expiry: time.Now().Add(-1 * time.Hour)}
	tokenSource := oauthConf.TokenSource(context.TODO(), expiredToken)
	return tokenSource.Token()
}

func tokenFromAuthCode(oauthConf *oauth2.Config) (*oauth2.Token, error) {
	// construct url and encode queries properly
	authURL := removeResponseTypeFromAuthURL(oauthConf.AuthCodeURL("", deviceName))

	// ask the user for the auth token
	fmt.Println("Can't find Refresh Token. Please navigate to the below address and paste the code")
	fmt.Println(authURL)
	fmt.Print("Auth Code: ")
	authCode := ""
	fmt.Scanln(&authCode)

	// got the auth_code. Exchange it with an access token
	token, err := oauthConf.Exchange(context.TODO(), authCode)
	return token, err
}

func removeResponseTypeFromAuthURL(uri string) string {
	auth, _ := url.Parse(uri)
	query := auth.Query()
	query.Del("response_type")
	auth.RawQuery = query.Encode()
	return auth.String()
}
