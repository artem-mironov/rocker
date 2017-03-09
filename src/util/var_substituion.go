/*-
 * Copyright 2017 jaspeen
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package util

import (
	"regexp"
	"strings"
)

var (
	// `\\\\+|[^\\]|\b|\A` - match any number of "\\" (ie, properly-escaped backslashes), or a single non-backslash character, or a word boundary, or beginning-of-line
	// `\$` - match literal $
	// `[[:alnum:]_]+` - match things like `$SOME_VAR`
	// `{[[:alnum:]_]+}` - match things like `${SOME_VAR}`
	tokenVarsInterpolation = regexp.MustCompile(`(\\|\\\\+|[^\\]|\b|\A)\$([[:alnum:]_]+|{[[:alnum:]_]+})`)
	// this intentionally punts on more exotic interpolations like ${SOME_VAR%suffix} and lets the shell handle those directly
)

// Replace variables in string using specified map
func ReplaceVars(vars map[string]string, str string) string {
	for _, match := range tokenVarsInterpolation.FindAllString(str, -1) {
		idx := strings.Index(match, "\\$")
		if idx != -1 {
			if idx+2 >= len(match) {
				str = strings.Replace(str, match, "\\$", -1)
				continue
			}

			prefix := match[:idx]
			stripped := match[idx+2:]
			str = strings.Replace(str, match, prefix+"$"+stripped, -1)
			continue
		}

		match = match[strings.Index(match, "$"):]
		matchKey := strings.Trim(match, "${}")

		val := vars[matchKey]
		str = strings.Replace(str, match, val, -1)
	}

	return str
}