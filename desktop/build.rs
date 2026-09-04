use cxx_qt_build::{CxxQtBuilder, QmlModule};
use qt_build_utils::QmlFile;

fn main() {
    // Cargo takes three version fields; a release identity has four. CI passes
    // the validated tag version in, and a build without it is a developer
    // build and says so.
    let crate_version = std::env::var("CARGO_PKG_VERSION").unwrap();
    let release_version = std::env::var("REDFLAG_RELEASE_VERSION")
        .ok()
        .filter(|v| !v.is_empty())
        .unwrap_or_else(|| crate_version.clone());
    // Valid identities are the crate version, or the crate version plus one
    // numeric field. The prefix carries its trailing dot, so this is a field
    // boundary and not a string prefix: 0.2.90.1 does not extend 0.2.9, and
    // neither does 0.2.9.3.4, 0.2.9. or 0.2.9.beta.
    let extends = match release_version.strip_prefix(&format!("{crate_version}.")) {
        Some(suffix) => !suffix.is_empty() && suffix.bytes().all(|b| b.is_ascii_digit()),
        None => release_version == crate_version,
    };
    if !extends {
        panic!("REDFLAG_RELEASE_VERSION={release_version} does not extend crate version {crate_version}");
    }
    println!("cargo:rustc-env=REDFLAG_VERSION={release_version}");
    println!("cargo:rerun-if-env-changed=REDFLAG_RELEASE_VERSION");

    let singletons = ["qml/Theme.qml"];
    let views = ["qml/LineGraph.qml", "qml/MetricCard.qml", "qml/NavItem.qml"];
    let mut files = vec![QmlFile::from("qml/Main.qml")];
    files.extend(
        singletons
            .iter()
            .map(|path| QmlFile::from(*path).singleton(true)),
    );
    files.extend(views.iter().map(|path| QmlFile::from(*path)));
    for path in ["qml/Main.qml"]
        .iter()
        .chain(singletons.iter())
        .chain(views.iter())
    {
        println!("cargo:rerun-if-changed={path}");
    }
    println!("cargo:rerun-if-changed=src/bridge/machine.rs");

    CxxQtBuilder::new_qml_module(QmlModule::new("com.redflag.desktop").qml_files(files))
        .qrc_resources(["icons/icon.png"])
        .file("src/bridge/machine.rs")
        .qt_module("Core")
        .qt_module("Gui")
        .qt_module("Qml")
        .qt_module("Quick")
        .build();
}
