use cxx_qt_lib::{QGuiApplication, QQmlApplicationEngine, QString, QUrl};

mod bridge;

fn main() {
    if std::env::args().any(|arg| arg == "--version" || arg == "-V") {
        println!("RedFlag v{}", env!("REDFLAG_VERSION"));
        return;
    }

    cxx_qt::init_crate!(redflag_desktop);
    cxx_qt::init_qml_module!("com.redflag.desktop");

    let mut app = QGuiApplication::new();
    if let Some(mut app) = app.as_mut() {
        app.as_mut()
            .set_application_name(&QString::from("com.redflag.Desktop"));
        app.as_mut()
            .set_application_display_name(&QString::from("RedFlag"));
        app.as_mut()
            .set_organization_name(&QString::from("RedFlag"));
    }

    let mut engine = QQmlApplicationEngine::new();
    if let Some(mut engine) = engine.as_mut() {
        engine
            .as_mut()
            .on_object_creation_failed(|_, url| {
                eprintln!("redflag-desktop: QML root failed to construct: {url}");
            })
            .release();
        engine.load(&QUrl::from("qrc:/qt/qml/com/redflag/desktop/qml/Main.qml"));
    }

    if let Some(app) = app.as_mut() {
        app.exec();
    }
}
