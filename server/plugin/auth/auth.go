/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package auth

import (
	"net/http"

	"github.com/apache/servicecomb-service-center/pkg/plugin"
)

const AUTH plugin.Kind = "auth"

type Authenticate interface {
	Identify(r *http.Request) error
	// ResourceScopes return the scope parsed from request
	// return nil mean apply all resources
	ResourceScopes(r *http.Request) []*ResourceScope
}

func Auth() Authenticate {
	return plugin.Plugins().Instance(AUTH).(Authenticate)
}

func Identify(r *http.Request) error {
	return Auth().Identify(r)
}

func ResourceScopes(r *http.Request) []*ResourceScope {
	return Auth().ResourceScopes(r)
}
