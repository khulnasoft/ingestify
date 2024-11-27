package publishers

import (
	"fmt"
	"time"

	"github.com/khulnasoft/ingestify/data"
)

type publishing struct {
	data.PackageVersion
	ttl time.Duration
}

func (p *publishing) Key() string {
	return fmt.Sprintf("ingestify:ingest:%s:%s:%s", p.Platform, p.Name, p.Version)
}
