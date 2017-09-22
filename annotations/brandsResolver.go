package annotations

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	log "github.com/sirupsen/logrus"
)

type Brand struct {
	ID          string  `json:"id"`
	ParentBrand *Brand  `json:"parentBrand"`
	ChildBrands []Brand `json:"childBrands"`
}

type BrandsResolverService interface {
	Refresh(brandUuids []string)
	GetBrands(brandUri string) []string
}

type brandsResolverService struct {
	sync.RWMutex
	brandsApiUrl string
	apiKey       string
	client       *http.Client
	idLinter     *IDLinter
	resolver     map[string][]string
}

func NewBrandsResolver(brandsApiUrl string, apiKey string, idLinter *IDLinter) BrandsResolverService {
	b := &brandsResolverService{
		brandsApiUrl: brandsApiUrl,
		apiKey:       apiKey,
		client:       http.DefaultClient,
		idLinter:     idLinter,
		resolver:     make(map[string][]string),
	}
	return b
}

func normalize(in []string) []string {
	out := map[string]struct{}{}

	for _, v := range in {
		out[v] = struct{}{}
	}

	return mapToSlice(out)
}

func mapToSlice(in map[string]struct{}) []string {
	out := make([]string, len(in))
	i := 0
	for k := range in {
		out[i] = k
		i++
	}

	return out
}

func (b *brandsResolverService) Refresh(brandURIs []string) {
	log.WithFields(log.Fields{"brandURIs": brandURIs, "source":b.brandsApiUrl}).Info("refresh brands")
	cleared := false
	for _, uri := range brandURIs {
		rootBrand, err := b.getBrand(uri)
		if err != nil {
			log.WithField("brandURI", uri).Error(err)
			continue
		}

		b.populateResolver(rootBrand, !cleared)
		cleared = true
	}

	log.WithField("count", len(b.resolver)).Info("brands loaded")
}

func (b *brandsResolverService) populateResolver(brand *Brand, clear bool) {
	b.Lock()
	defer b.Unlock()

	if clear {
		b.resolver = make(map[string][]string)
	}

	brand.ID = b.idLinter.Lint(brand.ID)
	resolved := map[string]struct{}{brand.ID: {}}
	if brand.ParentBrand != nil {
		parentBrand := b.idLinter.Lint(brand.ParentBrand.ID)
		resolved[parentBrand] = struct{}{}
		ancestors, found := b.resolver[parentBrand]
		if found {
			for _, ancestor := range ancestors {
				resolved[ancestor] = struct{}{}
			}
		}
	}

	resolvedBrands := mapToSlice(resolved)
	b.resolver[brand.ID] = resolvedBrands

	for _, child := range brand.ChildBrands {
		childBrand := b.idLinter.Lint(child.ID)
		b.resolver[childBrand] = append([]string{childBrand}, resolvedBrands...)
	}

	// attach these brands to any brands whose ancestor contains the resolved brand
	for uri, set := range b.resolver {
		contains := false
		for _, v := range set {
			if v == brand.ID {
				contains = true
				break
			}
		}

		if contains {
			b.resolver[uri] = normalize(append(b.resolver[uri], resolvedBrands...))
		}
	}
}

func (b *brandsResolverService) getBrand(brandURI string) (*Brand, error) {
	brandUUID := b.getBrandUUID(brandURI)
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(b.brandsApiUrl, brandUUID), nil)
	if err != nil {
		log.WithError(err).WithField("brandUUID", brandUUID).Error("unable to read brand")
		return nil, err
	}

	if b.apiKey != "" {
		req.Header.Add("X-Api-Key", b.apiKey)
	}

	resp, err := b.client.Do(req)
	if err != nil {
		log.WithError(err).WithField("brandUUID", brandUUID).Error("unable to read brand")
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.WithField("brandUUID", brandUUID).WithField("responseStatus", resp.StatusCode).Error("unable to read brand")
		return nil, errors.New("unable to read from brands API")
	}

	var brand Brand
	err = json.NewDecoder(resp.Body).Decode(&brand)
	if err != nil {
		log.WithError(err).WithField("brandUUID", brandUUID).Error("unable to deserialize brand")
		return nil, errors.New("unable to read response from brands API")
	}

	return &brand, nil
}

func (b *brandsResolverService) getBrandUUID(brandURI string) string {
	i := strings.LastIndex(brandURI, "/")
	return brandURI[i+1:]
}

func (b *brandsResolverService) GetBrands(brandURI string) []string {
	brands, found := b.resolveBrand(brandURI)
	if !found {
		brandToResolve := brandURI
		for {
			brand, err := b.getBrand(brandToResolve)
			if err != nil {
				return []string{}
			}

			b.populateResolver(brand, false)

			if brand.ParentBrand == nil {
				break
			}

			var resolved bool
			brandToResolve, resolved = b.isBrandResolved(b.idLinter.Lint(brand.ParentBrand.ID))
			if resolved {
				break
			}
		}

		brands, found = b.resolveBrand(brandURI)
		if !found {
			log.WithField("brandURI", brandURI).Warn("brand not found")
		}
	}

	return brands
}

func (b *brandsResolverService) resolveBrand(brandURI string) ([]string, bool) {
	b.RLock()
	defer b.RUnlock()

	brands, found := b.resolver[brandURI]
	return brands, found
}

func (b *brandsResolverService) isBrandResolved(brandURI string) (string, bool) {
	b.RLock()
	defer b.RUnlock()

	_, resolved := b.resolver[brandURI]
	return brandURI, resolved
}
