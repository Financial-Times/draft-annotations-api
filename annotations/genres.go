package annotations

import (
	"errors"
	"encoding/json"
	//"fmt"
	"net/http"
	"sync"

	log "github.com/sirupsen/logrus"
)

type Genre struct {
	ID string `json:"id"`
}

type GenresService interface {
	Refresh() ([]string, error)
	// genre can be a UUID or a URI
	IsGenre(conceptId string) bool
}

type genresService struct {
	sync.RWMutex
	genresApiUrl string
	apiKey       string
	client       *http.Client
	genres       map[string]struct{}
}

func NewGenresService(genresApiUrl string, apiKey string) GenresService {
	g := &genresService{
		genresApiUrl: genresApiUrl,
		apiKey:       apiKey,
		client:       http.DefaultClient,
		genres:       make(map[string]struct{}),
	}
	return g
}

// http://test.api.ft.com/concepts?type=http://www.ft.com/ontology/Genre

func (g *genresService) Refresh() ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, g.genresApiUrl, nil)
	if err != nil {
		log.WithError(err).Error("unable to read genres")
		return nil, err
	}

	if g.apiKey != "" {
		req.Header.Add("X-Api-Key", g.apiKey)
	}

	resp, err := g.client.Do(req)
	if err != nil {
		log.WithError(err).Error("unable to read genres")
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.WithField("responseStatus", resp.StatusCode).Error("unable to read genres")
		return nil, errors.New("unable to read genres")
	}

	var concepts map[string][]Genre
	err = json.NewDecoder(resp.Body).Decode(&concepts)
	if err != nil {
		log.WithError(err).Error("unable to deserialize genres")
		return nil, err
	}

	genres := []string{}
	for _, genre := range concepts["concepts"] {
		genres = append(genres, genre.ID)
	}

	g.populateGenres(genres)

	return genres, nil
}

func (g *genresService) populateGenres(genres []string) {
	g.Lock()
	defer g.Unlock()

	g.genres = make(map[string]struct{})

	for _, genre := range genres {
		g.genres[genre] = struct{}{}
	}
}

func (g *genresService) IsGenre(conceptId string) bool {
	_, found := g.genres[conceptId]
	return found
}
