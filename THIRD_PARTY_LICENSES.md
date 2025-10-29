# Third-Party Licenses

This document lists the third-party components and their licenses that are included in or required by RedFlag.

## Windows Update Package (Apache 2.0)

**Package**: `github.com/ceshihao/windowsupdate`
**Version**: Included as vendored code in `aggregator-agent/pkg/windowsupdate/`
**License**: Apache License 2.0
**Copyright**: Copyright 2022 Zheng Dayu
**Source**: https://github.com/ceshihao/windowsupdate
**License File**: https://github.com/ceshihao/windowsupdate/blob/main/LICENSE

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

RedFlag is licensed under the MIT License, which is compatible with the Apache License 2.0. Both are permissive open-source licenses that allow:

- Commercial use
- Modification
- Distribution
- Private use

The MIT license requires preservation of copyright notices, which is fulfilled through this attribution.