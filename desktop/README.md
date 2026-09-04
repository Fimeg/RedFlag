# RedFlag Desktop

Native Qt/QML local-machine operations console backed by the RedFlag Agent.

Desktop is the local scope of the RedFlag domain: machine health, live resources,
processes, connections, storage, services, containers, software, updates, security,
history, and bounded operator intent. RedFlag Web presents the same domain at fleet
scope.

The QML process holds no machine authority and reads no protected Agent state. Its
CXX-Qt bridge talks only to the platform-local Agent transport:

- Linux: `/var/lib/redflag/agent/localapi/redflag-agent.sock`
- Windows: `\\.\pipe\RedFlagAgentLocal`

Observation belongs to the Agent. Privileged mutations travel through Agent policy,
a signed mutation envelope, and the RedFlag helper. QML must never acquire a package,
service, container, or reboot escape hatch of its own.

The QML structure inherits useful interaction and typed-bridge work from Souveraine
Updater. That updater does not survive as a separate product or authority path.

The current release pipeline builds Desktop for Linux amd64 with Qt 6. The Windows
transport exists, but no Windows Desktop artifact is published until a native Qt/MSVC
runner owns that build. The web application remains the fleet surface; it is not
embedded into Desktop.
