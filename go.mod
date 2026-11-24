module github.com/bbernstein/lacylights-go

go 1.26.0

toolchain go1.27.4

require (
	github.com/99designs/gqlgen v0.17.84
	github.com/glebarez/sqlite v1.11.0
	github.com/go-chi/chi/v5 v5.5.0
	github.com/golang-jwt/jwt/v5 v5.3.1
	github.com/google/uuid v1.6.0
	// Untagged upstream main: v1.5.3 is affected by GO-2026-6278 and no fixed release is tagged yet.
	// TODO: switch back to a tagged release once gorilla/websocket v1.5.4 or later is published.
	github.com/gorilla/websocket v1.5.4-0.20250319132907-e064f32e3674
	github.com/joho/godotenv v1.5.1
	github.com/lucsky/cuid v1.2.1
	github.com/rs/cors v1.11.1
	github.com/stretchr/testify v1.11.1
	github.com/vektah/gqlparser/v2 v2.5.31
	github.com/xuri/excelize/v2 v2.11.1
	golang.org/x/crypto v0.59.0
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.31.1
)

require (
	github.com/agnivade/levenshtein v1.2.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/glebarez/go-sqlite v1.21.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/goccy/go-yaml v1.18.0 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/jinzhu/inflection v1.0.0 // indirect
	github.com/jinzhu/now v1.1.5 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-sqlite3 v1.14.22 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/richardlehane/mscfb v1.0.9 // indirect
	github.com/richardlehane/msoleps v1.0.6 // indirect
	github.com/sosodev/duration v1.3.1 // indirect
	github.com/tiendc/go-deepcopy v1.7.2 // indirect
	github.com/urfave/cli/v3 v3.6.1 // indirect
	github.com/xuri/efp v0.0.3-0.20260924042702-217b81952dec // indirect
	github.com/xuri/nfp v0.0.2-0.20250530014748-2ddeb826f9a9 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.63.0 // indirect
	golang.org/x/sync v0.25.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
	golang.org/x/text v0.44.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	modernc.org/libc v1.22.5 // indirect
	modernc.org/mathutil v1.5.0 // indirect
	modernc.org/memory v1.5.0 // indirect
	modernc.org/sqlite v1.23.1 // indirect
)
