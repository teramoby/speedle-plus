package eval

import (
	log "github.com/sirupsen/logrus"
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/store"
)

func (p *PolicyEvalImpl) Discover(ctx ads.RequestContext) (allowed bool, reason ads.Reason, err error) {
	defer func() {
		if _, recovered := err.(*recoveredEvaluationError); recovered {
			allowed = false
			reason = ads.ERROR_IN_EVALUATION
		}
	}()
	defer recoverEvaluation("Discover", &err)
	p.ensureStoreHealth()
	p.cacheGate.RLock()
	cacheErr := p.cacheErrorLocked()
	p.cacheGate.RUnlock()
	if cacheErr != nil {
		return false, ads.ERROR_IN_EVALUATION, cacheErr
	}
	if d, ok := p.Store.(store.DiscoverRequestManager); ok {
		// Discover requests are kept in the store; never keep the bearer token.
		if ctx.Subject != nil && ctx.Subject.Token != "" {
			subject := *ctx.Subject
			subject.Token = ""
			ctx.Subject = &subject
		}
		err := d.SaveDiscoverRequest(&ctx)
		if err != nil {
			log.Warn("error in saving discover request, ", err)
		}
		return true, ads.DISCOVER_MODE, err
	}
	return true, ads.DISCOVER_MODE, errors.Errorf(errors.DiscoverError, "unsupported store type of discovery function:%s", p.Store.Type())
}
