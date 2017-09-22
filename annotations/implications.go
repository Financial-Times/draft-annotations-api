package annotations

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
	tidutils "github.com/Financial-Times/transactionid-utils-go"
)

type Rule interface {
	Apply(ctx context.Context, ann []Annotation) ([]Annotation, error)
}

type Reasoner struct {
	rules []Rule
}

func NewReasoner(rules []Rule) *Reasoner {
	return &Reasoner{rules}
}

func (r *Reasoner) Process(ctx context.Context, ann []Annotation) ([]Annotation, error) {
	tid, err := tidutils.GetTransactionIDFromContext(ctx)
	if err != nil {
		tid = "not_found"
	}

	callLog := log.WithField(tidutils.TransactionIDKey, tid)

	for _, rule := range r.rules {
		callLog.WithField("rule", rule).Infof("applying rule on %d annotations", len(ann))
		var err error
		ann, err = rule.Apply(ctx, ann)
		if err != nil {
			return nil, err
		}
	}

	return ann, nil
}

type removeRule struct {
	predicates map[string]struct{}
}

func NewRemoveRule(predicates []string) Rule {
	p := map[string]struct{}{}
	for _, predicate := range predicates {
		p[predicate] = struct{}{}
	}

	return &removeRule{p}
}

func (r *removeRule) Apply(ctx context.Context, in []Annotation) ([]Annotation, error) {
	out := []Annotation{}

	for _, ann := range in {
		if _, remove := r.predicates[ann.Predicate]; remove {
			continue
		}

		out = append(out, ann)
	}

	return out, nil
}

func (r *removeRule) String() string {
	return fmt.Sprintf("remove predicates: %v", mapToSlice(r.predicates))
}

type implicitBrandsRule struct {
	predicates map[string]struct{}
	excludedConcepts ConceptChecker
	implicitPredicate string
	brandsResolverService BrandsResolverService
}

func NewImplicitBrandsRule(brandPredicates []string, implicitPredicate string, excludedConcepts ConceptChecker, brandsResolverService BrandsResolverService) Rule {
	p := map[string]struct{}{}
	for _, predicate := range brandPredicates {
		p[predicate] = struct{}{}
	}

	return &implicitBrandsRule{p, excludedConcepts, implicitPredicate, brandsResolverService}
}

func (r *implicitBrandsRule) Apply(ctx context.Context, in []Annotation) ([]Annotation, error) {
	out := []Annotation{}

	for _, ann := range in {
		out = append(out, ann)

		if _, isBrandPredicate := r.predicates[ann.Predicate]; !isBrandPredicate {
			continue
		}

		if r.excludedConcepts.IsConcept(ann.ConceptId) {
			continue
		}

		for _, brand := range r.brandsResolverService.GetBrands(ann.ConceptId) {
			if brand != ann.ConceptId {
				out = append(out, Annotation{Predicate:r.implicitPredicate, ConceptId:brand})
			}
		}
	}

	return out, nil
}

func (r *implicitBrandsRule) String() string {
	return fmt.Sprintf("add implicit brands with: %s", r.implicitPredicate)
}
