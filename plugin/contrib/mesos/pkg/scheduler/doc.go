/*
Copyright 2015 The Kubernetes Authors All rights reserved.

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

/*
Package framework includes a kubernetes framework.
that implements the interfaces of:

1: The mesos scheduler.

2: The kubernetes scheduler.

3: The kubernetes pod registry.

It acts as the 'scheduler' and the 'registry' of the PodRegistryStorage
to provide scheduling and Pod management on top of mesos.
*/
package scheduler
