package publishers

import "github.com/khulnasoft/ingestify/data"

type Publisher interface {
	Publish(data.PackageVersion)
}
