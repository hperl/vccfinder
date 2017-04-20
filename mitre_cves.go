package main

import (
	"bytes"
	"encoding/xml"
	"regexp"

	"code.google.com/p/go-charset/charset"
	_ "code.google.com/p/go-charset/data"
)

var repoMapping map[string]string

func init() {
	repoMapping = map[string]string{
		"qemu":              "bonzini/qemu",
		"php-src":           "php/php-src",
		"wireshark":         "wireshark",
		"ffmpeg":            "FFmpeg/FFmpeg",
		"libvirt":           "libvirt",
		"moodle":            "moodle/moodle",
		"libav":             "libav/libav",
		"quagga":            "quagga",
		"samba":             "samba",
		"openssl":           "openssl/openssl",
		"vlc":               "vlc",
		"vlc-1.0":           "vlc",
		"vlc-1.1":           "vlc",
		"chromium/chromium": "chromium/chromium",
		"xorg/lib/libX11":   "libX11",
		"glibc":             "glibc",
		//"dtc":               "dtc",
		"xen":  "xen",
		"flac": "flac",
	}
}

type Result struct {
	Vulnerabilities []Vulnerability `xml:"Vulnerability"`
}

type Vulnerability struct {
	CVE  string   `xml:"CVE"`
	URLs []string `xml:"References>Reference>URL"`
}

type MitreCves struct {
	data map[string](map[string]string) // map[repo][commit] = cve id
}

func NewMitreCves() *MitreCves {
	return &MitreCves{data: make(map[string](map[string]string))}
}

func (mc *MitreCves) Read(fname string) (err error) {
	var res Result
	githubRe := regexp.MustCompile(`^https://github.com/(\w+/\w+)/commit/(\w+)$`)
	gitRe := regexp.MustCompile(`^https?://.+/\?p=(.+).git;a=commit;h=(\w+)$`)
	linuxRe := regexp.MustCompile(`^linux/kernel`)

	file, err := Asset(fname)
	if err != nil {
		return
	}

	dec := xml.NewDecoder(bytes.NewReader(file))
	dec.CharsetReader = charset.NewReader
	if err = dec.Decode(&res); err != nil {
		return
	}

	// look for github urls in the vulnerabilites
	for _, vuln := range res.Vulnerabilities {
		for _, url := range vuln.URLs {
			var repo, sha string

			if m := githubRe.FindStringSubmatch(url); len(m) == 3 {
				repo = m[1]
				sha = m[2]
			} else if m := gitRe.FindStringSubmatch(url); len(m) == 3 {
				var ok bool
				repo, ok = repoMapping[m[1]]
				if !ok {
					if linuxRe.MatchString(m[1]) {
						repo = "torvalds/linux"
					} else {
						//fmt.Printf("no mapping found for %s (%s), consider adding it.\n", m[1], url)
					}
				}
				sha = m[2]
			}
			if repo != "" && sha != "" {
				if 7 < len(sha) && len(sha) < 40 {
					sha = (sha)[0:7]
				}
				if _, ok := mc.data[repo]; !ok {
					mc.data[repo] = make(map[string]string)
				}
				mc.data[repo][sha] = vuln.CVE
				continue
			}
		}
	}

	return nil
}

func (mc *MitreCves) Lookup(repo, sha string) (val string, ok bool) {
	// if repo doesn't exist return false
	if _, ok = mc.data[repo]; !ok {
		return "", false
	}
	if val, ok = mc.data[repo][sha]; !ok {
		val, ok = mc.data[repo][sha[0:7]]
	}

	return
}

func (mc *MitreCves) LookupCommit(c *Commit) (val string, ok bool) {
	return mc.Lookup(c.Repository.Name, c.Sha)
}

func (mc *MitreCves) Shas() (shas []string) {
	for repo, _ := range mc.data {
		shas = append(shas, mc.ShasForRepo(repo)...)
	}
	return
}

func (mc *MitreCves) ShasForRepo(repo string) (shas []string) {
	for sha, _ := range mc.data[repo] {
		shas = append(shas, sha)
	}
	return
}
