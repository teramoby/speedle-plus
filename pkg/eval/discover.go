package eval

import (
	"github.com/teramoby/speedle-plus/api/ads"
	"github.com/teramoby/speedle-plus/pkg/errors"
	"github.com/teramoby/speedle-plus/pkg/store"
	log "github.com/sirupsen/logrus"
)

func (p *PolicyEvalImpl) Discover(ctx ads.RequestContext) (bool, ads.Reason, error) {
	if d, ok := p.Store.(store.DiscoverRequestManager); ok {
		err := d.SaveDiscoverRequest(&ctx)
		if err != nil {
			log.Warn("error in saving discover request, ", err)
		}
		return true, ads.DISCOVER_MODE, err
	}
	return true, ads.DISCOVER_MODE, errors.Errorf(errors.DiscoverError, "unsupported store type of discovery function:%s", p.Store.Type())
}
