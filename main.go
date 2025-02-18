package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	tmdb "github.com/cyruzin/golang-tmdb"
	"github.com/gradleless/anime-sama-scraping/db" // sera pas donné pour des raisons de sécurité
	"github.com/gradleless/anime-sama-scraping/utils"
)

var validLecteurs = []string{"sendvid.com", "sibnet.ru", "vk.com", "vidmoly"}

func isValidLecteur(lecteur string) bool {
	for _, valid := range validLecteurs {
		if strings.Contains(lecteur, valid) {
			return true
		}
	}
	return false
}

func getProvider(str string) string {
	for _, valid := range validLecteurs {
		if strings.Contains(str, valid) {
			return valid
		}
	}
	return ""
}

func getLecteurFromName(name string) db.PlayerType {
	if strings.Contains(name, "sendvid.com") {
		return db.PlayerTypeSendvid
	} else if strings.Contains(name, "sibnet.ru") {
		return db.PlayerTypeSibnet
	} else if strings.Contains(name, "vk.com") {
		return db.PlayerTypeSibnet
	} else if strings.Contains(name, "vidmoly") {
		return db.PlayerTypeVidmoly
	}

	return db.PlayerType("unknown")
}

func main() {
	url := "https://anime-sama.fr/catalogue/listing_all.php"
	animes := ScrapeAnimeInfo(url)

	for _, anime := range animes {
		fmt.Printf("Name: %s\n", anime.Name)
		fmt.Printf("Href: %s\n", anime.Href)

		if !strings.Contains(anime.Name, "One Piece") {
			continue
		}

		var data *tmdb.TVDetails
		var err error
		for i := 0; i < 5; i++ {
			data, err = utils.SearchForShow(anime.Name, time.Now())
			if err == nil {
				break
			}
			log.Printf("Failed to search for %s: %s\n", anime.Name, err)
			time.Sleep(500 * time.Millisecond)
		}
		if err != nil {
			log.Printf("Skipping %s after 15 retries\n", anime.Name)
			continue
		}

		log.Printf("Found %s\n", data.Name)

		if !db.IsInDatabase(data.ID) {
		loopadd:
			_, err := db.AddAnimeByTVDetails(data)

			if err != nil {
				log.Println(err)
				time.Sleep(1300 * time.Millisecond)
				goto loopadd
			}

			log.Printf("Added %s to the database\n", data.Name)
		}

		db.UpdateAnimeByTVDetails(data)

		for si := 1; ; si++ {
			seasonURL := fmt.Sprintf("%s/saison%d/vostfr/episodes.js", anime.Href, si)
			seasons := ScrapeAnimeSeasons(seasonURL)
			if len(seasons) == 0 {
				break
			}

			if si >= 10 {
				continue
			}

			for part := 2; ; part++ {
				additionalSeasonURL := fmt.Sprintf("%s/saison%d-%d/vostfr/episodes.js", anime.Href, si, part)
				additionalSeasons := ScrapeAnimeSeasons(additionalSeasonURL)
				if len(additionalSeasons) == 0 {
					break
				}
				seasons = append(seasons, additionalSeasons...)
			}

			fmt.Printf("Season %d : %d episodes, Name: %s \n", si, len(seasons), anime.Name)

			var oldProvider string
			var currentEp int = 1

			var sibnetLinks []string
			var otherLinks []string
			var hasSibnetNext bool

			for i := 0; i < len(seasons); i++ {
				season := seasons[i]
				if strings.Contains(season, "sibnet") {
					if hasSibnetNext || i == len(seasons)-1 {
						sibnetLinks = append(sibnetLinks, season)
					}
					if i < len(seasons)-1 && strings.Contains(seasons[i+1], "sibnet") {
						hasSibnetNext = true
					} else {
						hasSibnetNext = false
					}
				} else {
					otherLinks = append(otherLinks, season)
				}
			}

			for _, season := range sibnetLinks {
				if oldProvider != getProvider(season) {
					currentEp = 1
					oldProvider = getProvider(season)
				}

				if db.HasEpisode(data.ID, si, currentEp) {
					fmt.Printf("Skipping S%dE%d: %s\n", si, currentEp, season)
					currentEp++
					continue
				}

			loopaddepSibnet:
				_, err = db.AddEpisode(data.ID, si, currentEp, season, string(getLecteurFromName(season)))
				if err != nil {
					log.Println(err)
					time.Sleep(1300 * time.Millisecond)
					goto loopaddepSibnet
				}

				fmt.Printf("Added S%dE%d: %s\n", si, currentEp, season)
				currentEp++
			}

			for _, season := range otherLinks {
				if oldProvider != getProvider(season) {
					currentEp = 1
					oldProvider = getProvider(season)
				}

				if db.HasEpisode(data.ID, si, currentEp) {
					fmt.Printf("Skipping S%dE%d: %s\n", si, currentEp, season)
					currentEp++
					continue
				}

			loopaddepOther:
				_, err = db.AddEpisode(data.ID, si, currentEp, season, string(getLecteurFromName(season)))
				if err != nil {
					log.Println(err)
					time.Sleep(1300 * time.Millisecond)
					goto loopaddepOther
				}

				fmt.Printf("Added S%dE%d: %s\n", si, currentEp, season)
				currentEp++
			}

			time.Sleep(50 * time.Millisecond)
		}
	}
}

type Anime struct {
	Name string
	Href string
}

func ScrapeAnimeInfo(url string) []Anime {
	response, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error making GET request: %s\n", err)
		return nil
	}
	defer response.Body.Close()

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %s\n", err)
		return nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		fmt.Printf("Error parsing HTML: %s\n", err)
		return nil
	}

	var animes []Anime
	doc.Find(".cardListAnime").Each(func(i int, s *goquery.Selection) {
		classes, _ := s.Attr("class")
		if strings.Contains(classes, "Anime") && strings.Contains(classes, "VOSTFR") {
			name := s.Find("h1").Text()
			href, _ := s.Find("a").Attr("href")
			anime := Anime{
				Name: name,
				Href: href,
			}
			animes = append(animes, anime)
		}
	})
	return animes
}

func ScrapeAnimeSeasons(url string) []string {
	response, err := http.Get(url)
	if err != nil {
		fmt.Printf("Error making GET request: %s\n", err)
		return nil
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		fmt.Printf("Error: %s\n", response.Status)
		return nil
	}

	body, err := ioutil.ReadAll(response.Body)
	if err != nil {
		fmt.Printf("Error reading response body: %s\n", err)
		return nil
	}

	lines := strings.Split(string(body), "\n")
	var episodes []string
	for _, line := range lines {
		if isValidLecteur(line) {
			matches := regexp.MustCompile(`['"](.*?)['"]`).FindAllStringSubmatch(line, -1)
			for _, match := range matches {
				episodes = append(episodes, match[1])
			}
		}
	}
	return episodes
}
