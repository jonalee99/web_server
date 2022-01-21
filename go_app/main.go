package main

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	// "encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/zmb3/spotify/v2"
	"github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

var (
	youtubeConf = &oauth2.Config{
		ClientID:     "1021942150636-k6uq1tma5r05p90ecebc1jscqfk5mi9c.apps.googleusercontent.com",
		ClientSecret: "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",
		RedirectURL:  "https://example.com",
		Scopes: []string{
			"https://www.googleapis.com/auth/userinfo.profile",
			"https://www.googleapis.com/auth/youtube.readonly",
		},
		Endpoint: google.Endpoint,
	}

	spotifyAuth = spotifyauth.New(
		spotifyauth.WithRedirectURL("http://localhost:8080/spotify/callback"),
		spotifyauth.WithScopes(spotifyauth.ScopePlaylistModifyPublic),
		spotifyauth.WithClientID("6fea87e058b543aebe2018417b462a9d"),
		spotifyauth.WithClientSecret("84c06f78bcfd458aa6e182435cef8313"),
	)

	// TODO: randomize it
	state = "pseudo-random"
)

type Song struct {
	Title string
	VideoOwnerChannelTitle string
}

type Inner struct {
	Snippet Song
}

type Outer struct {
	NextPageToken string
	Items []Inner
}

func main() {

	http.HandleFunc("/", handleMain)
	http.HandleFunc("/spotify", handleSpotifyLogin)
	http.HandleFunc("/spotify/callback", handleSpotifyCallback)
	http.HandleFunc("/youtube", handleYoutubeLogin)
	http.HandleFunc("/youtube/callback", handleYoutubeLogin)
	http.HandleFunc("/cookies", handleCookies)
	http.HandleFunc("/code", handleCode)
	fmt.Println("Server Running...")
	fmt.Println(http.ListenAndServe(":8080", nil))
}

func handleCode(w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.URL.Query().Get("code"))

	newURL, err := url.Parse(r.URL.Query().Get("code"))
	if err != nil {
		fmt.Println("error parsing url")
		return
	}

	// fmt.Println(newURL.Query().Get("code"))

	token, err := youtubeConf.Exchange(oauth2.NoContext, newURL.Query().Get("code"))
	if err != nil {
		fmt.Println("yt exchange failed")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	cookie := &http.Cookie{
		Name:   "youtubeAccessCode",
        Value:  token.AccessToken,
        MaxAge: int(token.Expiry.Sub(time.Now()).Seconds()),
		HttpOnly: true,
		Secure: true,
		Domain: "localhost",
		Path: "/",
	}
	http.SetCookie(w, cookie)
	fmt.Println("Great Success!")
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func getTitles(w http.ResponseWriter, r *http.Request, ytclient *http.Client, getinput string) (Outer, error) {
	resp, err := ytclient.Get(getinput)

	if err != nil {
		fmt.Println("reponse failed")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return Outer{}, err
	}

	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("error reading body")
		return Outer{}, err
	}
	// bodyString := string(bodyBytes)
	// fmt.Println(bodyString)

	
	var result Outer
	json.Unmarshal(bodyBytes, &result)

	return result, nil
}

func handleCookies(w http.ResponseWriter, r *http.Request) {
	// for _, c := range r.Cookies() {
	// 	fmt.Println("Cookie: " + c.String())
	// 	fmt.Println(c.Expires)
	// }

	spotifyCode, err := r.Cookie("spotifyAccessCode")
	if err != nil {
		fmt.Println("No cookie found")
		// http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		fmt.Fprintf(w, "You haven't logged into spotify")
		return
	}

	token := &oauth2.Token {
		AccessToken: spotifyCode.Value,
		TokenType: "Bearer",
	}

	spclient := spotify.New(spotifyAuth.Client(context.Background(), token))
	user, err := spclient.CurrentUser(context.Background())
	if err != nil {
		http.Error(w, "Couldn't get user", http.StatusNotFound)
		return
	}
	fmt.Println(user.DisplayName)

	// This is the youtube one
	youtubeCode, err := r.Cookie("youtubeAccessCode")
	if err != nil {
		// fmt.Println("No cookie found")
		// http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		fmt.Fprintf(w, "You haven't logged into youtube")
		return
	}

	token = &oauth2.Token {
		AccessToken: youtubeCode.Value,
		TokenType: "Bearer",
	}
	ytclient := youtubeConf.Client(context.Background(), token)
	// response, err := ytclient.Get("https://www.googleapis.com/youtube/v3/channels?part=contentDetails&mine=true")
	
	result, err := getTitles(w, r, ytclient, "https://www.googleapis.com/youtube/v3/playlistItems?playlistId=LM&part=snippet&maxResults=50")
	if err != nil {
		fmt.Println("error getting titles")
		return
	}

	// fmt.Println(result.Items[0].Snippet.Title)
	titles := make([]string, len(result.Items))
	m1 := regexp.MustCompile(`(\(.*\)|\[.*\]|OFFICIAL|M/V)`)
	m2 := regexp.MustCompile(`(VEVO|\s-\sTopic|music|Music)`)
	for i, x := range result.Items {
		// fmt.Println("Artist: " + x.Snippet.VideoOwnerChannelTitle)
		s1 := m1.ReplaceAllString(x.Snippet.Title, "")
		s1_split := strings.Split(s1, " - ")
		if len(s1_split) > 1 {
			titles[i] = strings.TrimSpace(s1_split[1]) + " " + strings.TrimSpace(s1_split[0])
		} else {
			titles[i] = strings.TrimSpace(s1) + " " + strings.TrimSpace(m2.ReplaceAllString(x.Snippet.VideoOwnerChannelTitle, ""))
		}
		
	}
	
	for result.NextPageToken != "" {
		// fmt.Println("Next page: " + result.NextPageToken)
		result, err = getTitles(w, r, ytclient, "https://www.googleapis.com/youtube/v3/playlistItems?playlistId=LM&part=snippet&maxResults=50&pageToken=" + result.NextPageToken)
		if err != nil {
			fmt.Println("error getting titles")
			return
		}
		temp1 := make([]string, len(result.Items))
		for i, x := range result.Items {
			// fmt.Println("Artist: " + x.Snippet.VideoOwnerChannelTitle)
			s1 := m1.ReplaceAllString(x.Snippet.Title, "")
			s1_split := strings.Split(s1, " - ")
			if len(s1_split) > 1 {
				temp1[i] = strings.TrimSpace(s1_split[1]) + " " + strings.TrimSpace(s1_split[0])
			} else {
				temp1[i] = strings.TrimSpace(s1) + " " + strings.TrimSpace(m2.ReplaceAllString(x.Snippet.VideoOwnerChannelTitle, ""))
			}
		}
		// fmt.Println(temp1[0])
		titles = append(titles, temp1...)
	}

	
	fmt.Println(len(titles))
	// for _, x := range titles {
	// 	fmt.Println(x)
	// }

	playlist, err := spclient.CreatePlaylistForUser(context.Background(), user.ID, "Youtube Liked", "This is a playlist with all your liked songs from youtube", true, false)
	if err != nil {
		fmt.Println("Error making playlist")
		return
	}

	for i, x := range titles {
		fmt.Println(i)
		query, err := spclient.Search(context.Background(), x, spotify.SearchTypeTrack)
		if err != nil {
			fmt.Println("Error search song: " + x)
			return
		}
		
		if len(query.Tracks.Tracks) > 0 {
			spclient.AddTracksToPlaylist(context.Background(), playlist.ID, query.Tracks.Tracks[0].ID)
		} else {
			fmt.Println(x)
		}
	}
}

func handleMain(w http.ResponseWriter, r *http.Request) {
	var htmlIndex = `
	<html>
	<body>
		<a href="/spotify">Spotify Log In</a></br>
		<a href="/youtube" target="_blank" rel="noopener noreferrer">Youtube Log In</a></br>
		<a href="/cookies">Put Liked Youtube Songs into Spotify Playlist</a>
		<form action="/code" method="get">
			<input type="text" id="code" name="code" placeholder="Enter Youtube Example.com Link here"/><br>
		</form>
	</body>
	</html>
	`

	fmt.Fprintf(w, htmlIndex)
}

func handleSpotifyLogin(w http.ResponseWriter, r *http.Request) {
	url := spotifyAuth.AuthURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleSpotifyCallback(w http.ResponseWriter, r *http.Request) {
	
	token, err := spotifyAuth.Token(r.Context(), state, r)
	if err != nil {
		http.Error(w, "Couldn't get token", http.StatusNotFound)
		return
	}

	// fmt.Println(time.Now())
	// fmt.Println(token.Expiry)
	// fmt.Println(int(token.Expiry.Sub(time.Now()).Seconds()))
	
	cookie := &http.Cookie{
		Name:   "spotifyAccessCode",
        Value:  token.AccessToken,
        MaxAge: int(token.Expiry.Sub(time.Now()).Seconds()),
		HttpOnly: true,
		Secure: true,
		Domain: "localhost",
		Path: "/",
	}
	http.SetCookie(w, cookie)
	client := spotify.New(spotifyAuth.Client(context.Background(), token))
	user, err := client.CurrentUser(context.Background())
	if err != nil {
		http.Error(w, "Couldn't get user", http.StatusNotFound)
		return
	}
	fmt.Println("Great Success: " + user.DisplayName)
	http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
}

func handleYoutubeLogin(w http.ResponseWriter, r *http.Request) {
	url := youtubeConf.AuthCodeURL(state)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

func handleYoutubeCallback(w http.ResponseWriter, r *http.Request) {
	fmt.Println("hi")

	if r.FormValue("state") != state {
		fmt.Println("invalid state")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	token, err := youtubeConf.Exchange(oauth2.NoContext, r.FormValue("code"))
	if err != nil {
		fmt.Println("exchange failed")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	cookie := &http.Cookie{
		Name:   "youtubeAccessCode",
        Value:  token.AccessToken,
        MaxAge: int(token.Expiry.Sub(time.Now()).Seconds()),
		HttpOnly: true,
		Secure: true,
		Domain: "localhost",
		Path: "/",
	}
	http.SetCookie(w, cookie)

	response, err := http.Get("https://www.googleapis.com/youtube/v3/playlistItems")
	if err != nil {
		fmt.Println("reponse failed")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	defer response.Body.Close()
	content, err := ioutil.ReadAll(response.Body)
	if err != nil {
		fmt.Println("reading failed")
		http.Redirect(w, r, "/", http.StatusTemporaryRedirect)
		return
	}

	fmt.Fprintf(w, "Content: %s\n", content)
}
