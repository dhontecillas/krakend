package proxy

import "strings"

// EntityFormatter formats the response data
type EntityFormatter interface {
	Format(entity Response) Response
}

// EntityFormatterFunc holds the formatter function
type EntityFormatterFunc struct {
	Func func(Response) Response
}

// Format implements the EntityFormatter interface
func (e EntityFormatterFunc) Format(entity Response) Response {
	return e.Func(entity)
}

type propertyFilter func(entity *Response)

type entityFormatter struct {
	Target         string
	Prefix         string
	PropertyFilter propertyFilter
	Mapping        map[string]string
}

// NewEntityFormatter creates an entity formatter with the received params
func NewEntityFormatter(target string, whitelist, blacklist []string, group string, mappings map[string]string) EntityFormatter {
	var propertyFilter propertyFilter
	if len(whitelist) > 0 {
		// propertyFilter = newWhitelistingFilter(whitelist)
		propertyFilter = newWhitelistFilterByDeletion(whitelist)
	} else {
		propertyFilter = newBlacklistingFilter(blacklist)
	}
	sanitizedMappings := make(map[string]string, len(mappings))
	for i, m := range mappings {
		v := strings.Split(m, ".")
		sanitizedMappings[i] = v[0]
	}
	return entityFormatter{
		Target:         target,
		Prefix:         group,
		PropertyFilter: propertyFilter,
		Mapping:        sanitizedMappings,
	}
}

// Format implements the EntityFormatter interface
func (e entityFormatter) Format(entity Response) Response {
	if e.Target != "" {
		extractTarget(e.Target, &entity)
	}
	if len(entity.Data) > 0 {
		e.PropertyFilter(&entity)
	}
	if len(entity.Data) > 0 {
		for formerKey, newKey := range e.Mapping {
			if v, ok := entity.Data[formerKey]; ok {
				entity.Data[newKey] = v
				delete(entity.Data, formerKey)
			}
		}
	}
	if e.Prefix != "" {
		entity.Data = map[string]interface{}{e.Prefix: entity.Data}
	}
	return entity
}

func extractTarget(target string, entity *Response) {
	if tmp, ok := entity.Data[target]; ok {
		entity.Data, ok = tmp.(map[string]interface{})
		if !ok {
			entity.Data = map[string]interface{}{}
		}
	} else {
		entity.Data = map[string]interface{}{}
	}
}

func newWhiteListDict(whitelist []string) map[string]interface{} {
	wlDict := make(map[string]interface{})
	for _, k := range whitelist {
		wlFields := strings.Split(k, ".")
		d := buildDictPath(wlDict, wlFields[:len(wlFields)-1])
		d[wlFields[len(wlFields)-1]] = true
	}
	return wlDict
}

func whitelistByDeletionPrune(wlDict map[string]interface{}, inDict map[string]interface{}) bool {
	canDelete := true
	for k, v := range inDict {
		if subWl, ok := wlDict[k]; ok {
			if subWlDict, okk := subWl.(map[string]interface{}); okk {
				if subInDict, isDict := v.(map[string]interface{}); isDict {
					if !whitelistByDeletionPrune(subWlDict, subInDict) {
						canDelete = false
					} else {
						delete(inDict, k)
					}
				} else {
					delete(inDict, k)
				}
			} else {
				// we found the whitelist leaf, and should maintain this branch
				canDelete = false
			}
		} else {
			delete(inDict, k)
		}
	}
	return canDelete
}

func newWhitelistFilterByDeletion(whitelist []string) propertyFilter {
	wlDict := newWhiteListDict(whitelist)

	return func(entity *Response) {
		if whitelistByDeletionPrune(wlDict, entity.Data) {
			for k := range entity.Data {
				delete(entity.Data, k)
			}
		}
	}
}

func newWhitelistingFilter(whitelist []string) propertyFilter {
	numFields := 0
	for _, k := range whitelist {
		numFields += len(strings.Split(k, "."))
	}
	wlIndices := make([]int, len(whitelist))
	wlFields := make([]string, numFields)
	fIdx := 0
	for wIdx, k := range whitelist {
		for _, key := range strings.Split(k, ".") {
			wlFields[fIdx] = key
			fIdx++
		}
		wlIndices[wIdx] = fIdx
	}

	return func(entity *Response) {
		accumulator := make(map[string]interface{}, len(whitelist))
		start := 0
		for _, end := range wlIndices {
			dEnd := end - 1
			p := findDictPath(entity.Data, wlFields[start:dEnd])
			if value, ok := p[wlFields[dEnd]]; ok {
				d := buildDictPath(accumulator, wlFields[start:dEnd])
				d[wlFields[dEnd]] = value
			}
			start = end
		}
		*entity = Response{Data: accumulator, IsComplete: entity.IsComplete}
	}
}

func findDictPath(root map[string]interface{}, fields []string) map[string]interface{} {
	ok := true
	p := root
	for _, field := range fields {
		if p, ok = p[field].(map[string]interface{}); !ok {
			return nil
		}
	}
	return p
}

func buildDictPath(accumulator map[string]interface{}, fields []string) map[string]interface{} {
	var ok bool = true
	var c map[string]interface{}
	var fIdx int
	fEnd := len(fields)
	p := accumulator
	for fIdx = 0; fIdx < fEnd; fIdx++ {
		if c, ok = p[fields[fIdx]].(map[string]interface{}); !ok {
			break
		}
		p = c
	}
	for ; fIdx < fEnd; fIdx++ {
		c = make(map[string]interface{})
		p[fields[fIdx]] = c
		p = c
	}
	return p
}

func newBlacklistingFilter(blacklist []string) propertyFilter {
	bl := make(map[string][]string, len(blacklist))
	for _, key := range blacklist {
		keys := strings.Split(key, ".")
		if len(keys) > 1 {
			if sub, ok := bl[keys[0]]; ok {
				bl[keys[0]] = append(sub, keys[1])
			} else {
				bl[keys[0]] = []string{keys[1]}
			}
		} else {
			bl[keys[0]] = []string{}
		}
	}

	return func(entity *Response) {
		for k, sub := range bl {
			if len(sub) == 0 {
				delete(entity.Data, k)
			} else {
				if tmp := blacklistFilterSub(entity.Data[k], sub); len(tmp) > 0 {
					entity.Data[k] = tmp
				}
			}
		}
	}
}

func blacklistFilterSub(v interface{}, blacklist []string) map[string]interface{} {
	tmp, ok := v.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	for _, key := range blacklist {
		delete(tmp, key)
	}
	return tmp
}
