# Third-Party Licenses

This document lists the third-party components and their licenses that are included in or required by RedFlag.

## Windows Update Package (Apache 2.0)

**Package**: `github.com/ceshihao/windowsupdate`
**Version**: Vendored from master branch (Jan 2026 refactor), in `agent/pkg/windowsupdate/`
**License**: Apache License 2.0
**Copyright**: Copyright 2022 Zheng Dayu
**Source**: https://github.com/ceshihao/windowsupdate
**License File**: https://github.com/ceshihao/windowsupdate/blob/master/LICENSE

### License Text

```
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
```

### Modifications

The package has been modified for integration with RedFlag's update management system. Modifications include:

- Integration with RedFlag's update reporting format
- Added support for RedFlag's metadata structures
- Compatibility with RedFlag's agent communication protocol

All modifications maintain the original Apache 2.0 license.

---

## License Compatibility

RedFlag is licensed under the GNU Affero General Public License v3.0 (AGPL-3.0). The Apache License 2.0 components (windowsupdate) are compatible with AGPL-3.0, as Apache 2.0 is listed by the FSF as compatible with GPLv3 and later.

The windowsupdate package retains its original Apache 2.0 license. This attribution fulfills the requirements of both licenses.