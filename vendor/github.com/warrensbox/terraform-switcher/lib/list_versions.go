package lib

import (
	"io/ioutil"
	"log"
	"net/http"
	"reflect"
	"regexp"
	"strings"
)

type tfVersionList struct {
	tflist []string
}

//GetTFList :  Get the list of available terraform version given the hashicorp url
func GetTFList(hashiURL string, listAll bool) ([]string, error) {

	/* Get list of terraform versions from hashicorp releases */
	resp, errURL := http.Get(hashiURL)
	if errURL != nil {
		log.Printf("Error getting url: %v", errURL)
		return nil, errURL
	}
	defer resp.Body.Close()

	body, errBody := ioutil.ReadAll(resp.Body)
	if errBody != nil {
		log.Printf("Error reading body: %v", errBody)
		return nil, errBody
	}

	bodyString := string(body)
	result := strings.Split(bodyString, "\n")

	var tfVersionList tfVersionList

	for i := range result {
		// Getting versions from body; should return match /X.X.X/ where X is a number
		// Follow https://semver.org/spec/v2.0.0.html
		r, _ := regexp.Compile(`\/(\d+\.\d+\.\d+)\/`)
		if listAll {
			// Getting versions from body; should return match /X.X.X-@/ where X is a number,@ is a word character between a-z or A-Z
			// Follow https://semver.org/spec/v1.0.0-beta.html
			// Check regular expression at https://rubular.com/r/ju3PxbaSBALpJB
			r, _ = regexp.Compile(`\/(\d+\.\d+\.\d+)(-[a-zA-z]+\d*)?\/`)
		}

		if r.MatchString(result[i]) {
			str := r.FindString(result[i])
			trimstr := strings.Trim(str, "/") //remove "/" from /X.X.X/
			tfVersionList.tflist = append(tfVersionList.tflist, trimstr)
		}
	}

	return tfVersionList.tflist, nil

}

//VersionExist : check if requested version exist
func VersionExist(val interface{}, array interface{}) (exists bool) {

	exists = false
	switch reflect.TypeOf(array).Kind() {
	case reflect.Slice:
		s := reflect.ValueOf(array)

		for i := 0; i < s.Len(); i++ {
			if reflect.DeepEqual(val, s.Index(i).Interface()) == true {
				exists = true
				return exists
			}
		}
	}

	return exists
}

//RemoveDuplicateVersions : remove duplicate version
func RemoveDuplicateVersions(elements []string) []string {
	// Use map to record duplicates as we find them.
	encountered := map[string]bool{}
	result := []string{}

	for _, val := range elements {
		versionOnly := strings.Trim(val, " *recent")
		if encountered[versionOnly] == true {
			// Do not add duplicate.
		} else {
			// Record this element as an encountered element.
			encountered[versionOnly] = true
			// Append to result slice.
			result = append(result, val)
		}
	}
	// Return the new slice.
	return result
}

// ValidVersionFormat : returns valid version format
/* For example: 0.1.2 = valid
// For example: 0.1.2-beta1 = valid
// For example: 0.1.2-alpha = valid
// For example: a.1.2 = invalid
// For example: 0.1. 2 = invalid
*/
func ValidVersionFormat(version string) bool {

	// Getting versions from body; should return match /X.X.X-@/ where X is a number,@ is a word character between a-z or A-Z
	// Follow https://semver.org/spec/v1.0.0-beta.html
	// Check regular expression at https://rubular.com/r/ju3PxbaSBALpJB
	semverRegex := regexp.MustCompile(`^(\d+\.\d+\.\d+)(-[a-zA-z]+\d*)?$`)

	return semverRegex.MatchString(version)
}
