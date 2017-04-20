package main

import (
	"log"
	"testing"
)

type td struct {
	repo string
	sha  string
	cve  string
}

func TestMitreCveCommit(t *testing.T) {
	data := []td{
		td{"krb5/krb5", "cf1a0c411b2668c57c41e9c4efd15ba17b6b322c", "CVE-2002-2443"},
		td{"npm/npm", "f4d31693e73a963574a88000580db1a716fe66f1", "CVE-2013-4116"},
		td{"bagder/curl", "75ca568fa1c19de4c5358fed246686de8467c238", "CVE-2012-0036"},
		td{"openssl/openssl", "26a59d9b46574e457870197dffa802871b4c8fc7", "CVE-2014-3568"},
		td{"openssl/openssl", "96db9023b881d7cd9f379b0c154650d6c108e9a3", "CVE-2014-0160"},
		td{"torvalds/linux", "350b8bdd689cd2ab2c67c8a86a0be86cfa0751a7", "CVE-2014-3601"},
		td{"bonzini/qemu", "ab9509cceabef28071e41bdfa073083859c949a7", "CVE-2014-3615"},
		td{"php/php-src", "88412772d295ebf7dd34409534507dc9bcac726e", "CVE-2014-3668"},
		td{"torvalds/linux", "082d52c56f642d21b771a13221068d40915a1409", "CVE-2005-3783"},
	}

	cves := NewMitreCves()
	if err := cves.Read("data/cve.xml"); err != nil {
		log.Fatal(err)
	}

	for _, d := range data {
		cve, ok := cves.Lookup(d.repo, d.sha)
		if !ok || cve != d.cve {
			t.Errorf("%s %s: expected '%s', got '%s'.", d.repo, d.sha, d.cve, cve)
		}
	}
}
