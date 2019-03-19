package yamlsearch

import "fmt"

// FindValue will return the found value or blank, from the list of keys and a given yaml
func FindValue(what map[interface{}]interface{}, wantedPath []string) (wantedValue string) {
	return search(what, nil, wantedPath)
}

func search(what interface{}, previous []string, wantedPath []string) (wantedValue string) {

	if previous != nil && len(previous) > 0 {
		if previous[len(previous)-1] != wantedPath[len(previous)-1] {
			return ""
		} else if len(previous) == len(wantedPath) && previous[len(previous)-1] == wantedPath[len(wantedPath)-1] {
			return fmt.Sprintf("%v", what)
		}
	}

	if value, ok := what.(map[interface{}]interface{}); ok {
		for key, value2 := range value {
			var myprevious []string

			if previous == nil {
				myprevious = []string{}
			} else {
				myprevious = previous
			}

			myprevious = append(myprevious, key.(string))

			wantedValue = search(value2, myprevious, wantedPath)

			if wantedValue != "" {
				return wantedValue
			}
		}
	} else if value, ok := what.([]interface{}); ok {
		for _, value2 := range value {
			wantedValue = search(value2, previous, wantedPath)

			if wantedValue != "" {
				return wantedValue
			}
		}
	}

	return ""
}
