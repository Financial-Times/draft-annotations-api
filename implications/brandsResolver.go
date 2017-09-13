package implications

import (
	"net/http"
	"strings"
	"sync"

	"encoding/json"
	"errors"
	log "github.com/sirupsen/logrus"
	"fmt"
)

type Brand struct {
	ID          string  `json:"id"`
	ParentBrand *Brand  `json:"parentBrand"`
	ChildBrands []Brand `json:"childBrands"`
}

type BrandsResolverService interface {
	Refresh(brandUuids []string)
	GetBrands(brandUuid string) []string
}

type brandsResolverService struct {
	sync.RWMutex
	brandsApiUrl string
	apiKey       string
	client       *http.Client
	resolver     map[string][]string
}

func NewBrandsResolver(brandsApiUrl string, apiKey string) BrandsResolverService {
	b := &brandsResolverService{
		brandsApiUrl: brandsApiUrl,
		apiKey:       apiKey,
		client:       http.DefaultClient,
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

func (b *brandsResolverService) Refresh(brandUUIDs []string) {
	cleared := false
	for _, uuid := range brandUUIDs {
		rootBrand, err := b.getBrand(uuid)
		if err != nil {
			log.WithField("brandUUID", uuid).Error(err)
			continue
		}

		b.populateResolver(rootBrand, !cleared)
		cleared = true
	}
}

func (b *brandsResolverService) populateResolver(brand *Brand, clear bool) {
	b.Lock()
	defer b.Unlock()

	if clear {
		b.resolver = make(map[string][]string)
	}

	brandUUID := b.getBrandUUID(brand.ID)
	resolved := map[string]struct{}{brandUUID: {}}
	if brand.ParentBrand != nil {
		parentBrand := b.getBrandUUID(brand.ParentBrand.ID)
		resolved[parentBrand] = struct{}{}
		ancestors, found := b.resolver[parentBrand]
		if found {
			for _, ancestor := range ancestors {
				resolved[ancestor] = struct{}{}
			}
		}
	}

	resolvedBrands := mapToSlice(resolved)
	b.resolver[brandUUID] = resolvedBrands

	for _, child := range brand.ChildBrands {
		childBrand := b.getBrandUUID(child.ID)
		b.resolver[childBrand] = append([]string{childBrand}, resolvedBrands...)
	}

	// attach these brands to any brands whose ancestor contains the resolved brand
	for uuid, set := range b.resolver {
		contains := false
		for _, v := range set {
			if v == brandUUID {
				contains = true
				break
			}
		}

		if contains {
			b.resolver[uuid] = normalize(append(b.resolver[uuid], resolvedBrands...))
		}
	}
}

func (b *brandsResolverService) getBrand(brandUUID string) (*Brand, error) {
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

func (b *brandsResolverService) GetBrands(brandUUID string) []string {
	brands, found := b.resolveBrand(brandUUID)
	if !found {
		brandToResolve := brandUUID
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
			brandToResolve, resolved = b.isBrandResolved(brand.ParentBrand.ID)
			if resolved {
				break
			}
		}

		brands, found = b.resolveBrand(brandUUID)
		if !found {
			log.WithField("brandUUID", brandUUID).Warn("brand not found")
		}
	}

	return brands
}

func (b *brandsResolverService) resolveBrand(brandUuid string) ([]string, bool) {
	b.RLock()
	defer b.RUnlock()

	brands, found := b.resolver[brandUuid]
	return brands, found
}

func (b *brandsResolverService) isBrandResolved(brandURI string) (string, bool) {
	b.RLock()
	defer b.RUnlock()

	brandUUID := b.getBrandUUID(brandURI)
	_, resolved := b.resolver[brandUUID]
	return brandUUID, resolved
}
