module github.com/horchestra/pack-ui

go 1.27.1

require (
	github.com/go-git/go-git/v6 v6.0.0-beta.1
	github.com/horchestra/pack-operator v0.0.0
	github.com/rs/zerolog v1.35.1
	sigs.k8s.io/yaml v1.6.0
)

require (
	github.com/Microsoft/go-winio v0.6.3-0.20251027160822-ad3df93bed29 // indirect
	github.com/ProtonMail/go-crypto v1.5.2 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/dustin/go-humanize v1.1.0 // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/go-git/gcfg/v2 v2.0.2 // indirect
	github.com/go-git/go-billy/v6 v6.0.0-beta.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.12.0 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/kevinburke/ssh_config v1.6.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.1.0 // indirect
	github.com/pjbgf/sha1cd v0.7.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/sergi/go-diff v1.4.0 // indirect
	go.yaml.in/yaml/v2 v2.4.4 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	modernc.org/libc v1.79.0 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.61.1 // indirect
)

replace github.com/horchestra/pack-operator => ../operator

replace github.com/horchestra/pack => ../

replace github.com/horchestra/pack/sdk => ../sdk

replace github.com/arenadata/oci-packer => github.com/arenadata/oci-packer v0.0.0-20261102154302-3f039c44a9be

replace github.com/arenadata/oci-packer/types => github.com/arenadata/oci-packer/types v0.0.0-20261102154302-3f039c44a9be
