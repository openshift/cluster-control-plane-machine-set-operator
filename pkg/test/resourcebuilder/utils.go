/*
Copyright 2022 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package resourcebuilder

// boolPtr returns a pointer to the bool value.
func boolPtr(b bool) *bool {
	return &b
}

// int32Ptr returns a pointer to the int32 value.
func int32Ptr(i int32) *int32 {
	return &i
}

// int64Ptr returns a pointer to the int64 value.
func int64Ptr(i int64) *int64 {
	return &i
}

// stringPtr returns a pointer to the string value.
func stringPtr(s string) *string {
	return &s
}
