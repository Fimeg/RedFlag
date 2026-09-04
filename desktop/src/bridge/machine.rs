use super::local_api;
use cxx_qt::Threading;
use cxx_qt_lib::QString;
use serde_json::Value;
use std::pin::Pin;
use std::sync::LazyLock;
use std::time::Instant;

static DESKTOP_START: LazyLock<Instant> = LazyLock::new(Instant::now);

#[cxx_qt::bridge]
pub mod ffi {
    unsafe extern "C++" {
        include!("cxx-qt-lib/qstring.h");
        type QString = cxx_qt_lib::QString;
    }

    #[auto_cxx_name]
    unsafe extern "RustQt" {
        #[qobject]
        #[qml_element]
        #[qproperty(bool, connected)]
        #[qproperty(QString, connection_error)]
        #[qproperty(QString, agent_gap)]
        #[qproperty(QString, hostname)]
        #[qproperty(QString, machine_subtitle)]
        #[qproperty(QString, agent_version)]
        #[qproperty(QString, agent_status)]
        #[qproperty(QString, enrollment)]
        #[qproperty(QString, uptime)]
        #[qproperty(f64, cpu_usage)]
        #[qproperty(f64, load_1)]
        #[qproperty(f64, load_5)]
        #[qproperty(f64, load_15)]
        #[qproperty(f64, memory_percent)]
        #[qproperty(u64, memory_used)]
        #[qproperty(u64, memory_total)]
        #[qproperty(f64, swap_percent)]
        #[qproperty(f64, network_receive)]
        #[qproperty(f64, network_transmit)]
        #[qproperty(f64, disk_read)]
        #[qproperty(f64, disk_write)]
        #[qproperty(f64, temperature)]
        #[qproperty(i32, process_count)]
        #[qproperty(i32, update_count)]
        #[qproperty(i32, critical_count)]
        #[qproperty(i32, container_count)]
        #[qproperty(i32, container_running)]
        #[qproperty(i32, container_unhealthy)]
        #[qproperty(i32, service_count)]
        #[qproperty(i32, service_running)]
        #[qproperty(i32, service_failed)]
        #[qproperty(QString, health_state)]
        #[qproperty(QString, history_json)]
        #[qproperty(QString, system_json)]
        #[qproperty(QString, processes_json)]
        #[qproperty(QString, process_detail_json)]
        #[qproperty(QString, software_json)]
        #[qproperty(QString, software_detail_json)]
        #[qproperty(QString, connections_json)]
        #[qproperty(QString, containers_json)]
        #[qproperty(QString, services_json)]
        #[qproperty(QString, updates_json)]
        #[qproperty(QString, security_json)]
        #[qproperty(QString, events_json)]
        #[qproperty(QString, operation_message)]
        #[qproperty(QString, approval_json)]
        #[qproperty(i32, software_count)]
        #[qproperty(i32, software_explicit_count)]
        #[qproperty(i32, software_dependency_count)]
        #[qproperty(i32, software_foreign_count)]
        #[qproperty(bool, approval_running)]
        #[qproperty(bool, telemetry_loading)]
        #[qproperty(bool, overview_loading)]
        #[qproperty(bool, processes_loading)]
        #[qproperty(bool, software_loading)]
        type Machine = super::MachineRust;

        #[qinvokable]
        fn refresh_telemetry(self: Pin<&mut Machine>);
        #[qinvokable]
        fn refresh_overview(self: Pin<&mut Machine>);
        #[qinvokable]
        fn refresh_processes(self: Pin<&mut Machine>);
        #[qinvokable]
        fn refresh_connections(self: Pin<&mut Machine>);
        #[qinvokable]
        fn load_process(self: Pin<&mut Machine>, pid: i32);
        #[qinvokable]
        fn refresh_software(self: Pin<&mut Machine>);
        #[qinvokable]
        fn load_software(self: Pin<&mut Machine>, package_type: &QString, identity: &QString);
        #[qinvokable]
        fn trigger_scan(self: Pin<&mut Machine>);
        #[qinvokable]
        fn approve_update(
            self: Pin<&mut Machine>,
            package_type: &QString,
            package_name: &QString,
            available_version: &QString,
            override_reason: &QString,
        );

        #[qsignal]
        fn telemetry_updated(self: Pin<&mut Machine>);
        #[qsignal]
        fn overview_updated(self: Pin<&mut Machine>);
        #[qsignal]
        fn processes_updated(self: Pin<&mut Machine>);
        #[qsignal]
        fn connections_updated(self: Pin<&mut Machine>);
        #[qsignal]
        fn process_detail_updated(self: Pin<&mut Machine>);
        #[qsignal]
        fn software_updated(self: Pin<&mut Machine>);
        #[qsignal]
        fn software_detail_updated(self: Pin<&mut Machine>);
        #[qsignal]
        fn operation_finished(self: Pin<&mut Machine>, success: bool);
        #[qsignal]
        fn approval_finished(self: Pin<&mut Machine>, success: bool);
    }

    impl cxx_qt::Threading for Machine {}
}

pub struct MachineRust {
    connected: bool,
    connection_error: QString,
    agent_gap: QString,
    hostname: QString,
    machine_subtitle: QString,
    agent_version: QString,
    agent_status: QString,
    enrollment: QString,
    uptime: QString,
    cpu_usage: f64,
    load_1: f64,
    load_5: f64,
    load_15: f64,
    memory_percent: f64,
    memory_used: u64,
    memory_total: u64,
    swap_percent: f64,
    network_receive: f64,
    network_transmit: f64,
    disk_read: f64,
    disk_write: f64,
    temperature: f64,
    process_count: i32,
    update_count: i32,
    critical_count: i32,
    container_count: i32,
    container_running: i32,
    container_unhealthy: i32,
    service_count: i32,
    service_running: i32,
    service_failed: i32,
    health_state: QString,
    history_json: QString,
    system_json: QString,
    processes_json: QString,
    process_detail_json: QString,
    software_json: QString,
    software_detail_json: QString,
    connections_json: QString,
    containers_json: QString,
    services_json: QString,
    updates_json: QString,
    security_json: QString,
    events_json: QString,
    operation_message: QString,
    approval_json: QString,
    software_count: i32,
    software_explicit_count: i32,
    software_dependency_count: i32,
    software_foreign_count: i32,
    approval_running: bool,
    telemetry_loading: bool,
    overview_loading: bool,
    processes_loading: bool,
    software_loading: bool,
}

impl Default for MachineRust {
    fn default() -> Self {
        Self {
            connected: false,
            connection_error: QString::default(),
            agent_gap: QString::default(),
            hostname: QString::from("This machine"),
            machine_subtitle: QString::from("Waiting for RedFlag Agent"),
            agent_version: QString::default(),
            agent_status: QString::from("connecting"),
            enrollment: QString::from("unknown"),
            uptime: QString::default(),
            cpu_usage: 0.0,
            load_1: 0.0,
            load_5: 0.0,
            load_15: 0.0,
            memory_percent: 0.0,
            memory_used: 0,
            memory_total: 0,
            swap_percent: 0.0,
            network_receive: 0.0,
            network_transmit: 0.0,
            disk_read: 0.0,
            disk_write: 0.0,
            temperature: 0.0,
            process_count: 0,
            update_count: 0,
            critical_count: 0,
            container_count: 0,
            container_running: 0,
            container_unhealthy: 0,
            service_count: 0,
            service_running: 0,
            service_failed: 0,
            health_state: QString::from("observing"),
            history_json: QString::from("[]"),
            system_json: QString::from("{}"),
            processes_json: QString::from("[]"),
            process_detail_json: QString::from("{}"),
            software_json: QString::from("{}"),
            software_detail_json: QString::from("{}"),
            connections_json: QString::from("[]"),
            containers_json: QString::from("[]"),
            services_json: QString::from("[]"),
            updates_json: QString::from("[]"),
            security_json: QString::from("{}"),
            events_json: QString::from("[]"),
            operation_message: QString::default(),
            approval_json: QString::from("{}"),
            software_count: 0,
            software_explicit_count: 0,
            software_dependency_count: 0,
            software_foreign_count: 0,
            approval_running: false,
            telemetry_loading: false,
            overview_loading: false,
            processes_loading: false,
            software_loading: false,
        }
    }
}

impl ffi::Machine {
    pub fn refresh_telemetry(mut self: Pin<&mut Self>) {
        if self.telemetry_loading {
            return;
        }
        self.as_mut().set_telemetry_loading(true);
        let thread = self.qt_thread();
        std::thread::spawn(move || {
            let result = match local_api::get_optional("/v1/monitor") {
                Ok(Some(value)) => Ok(value),
                Ok(None) => Ok(Value::Object(serde_json::Map::new())),
                Err(error) => Err(error),
            };
            let _ = thread.queue(move |machine| machine.apply_telemetry(result));
        });
    }

    pub fn refresh_overview(mut self: Pin<&mut Self>) {
        if self.overview_loading {
            return;
        }
        self.as_mut().set_overview_loading(true);
        let thread = self.qt_thread();
        std::thread::spawn(move || {
            let result = load_overview();
            let _ = thread.queue(move |machine| machine.apply_overview(result));
        });
    }

    pub fn refresh_processes(mut self: Pin<&mut Self>) {
        if self.processes_loading {
            return;
        }
        self.as_mut().set_processes_loading(true);
        let thread = self.qt_thread();
        std::thread::spawn(move || {
            let result = local_api::get("/v1/processes");
            let _ = thread.queue(move |machine| machine.apply_processes(result));
        });
    }

    pub fn refresh_connections(self: Pin<&mut Self>) {
        let thread = self.qt_thread();
        std::thread::spawn(move || {
            let result = local_api::get("/v1/connections");
            let _ = thread.queue(move |machine| machine.apply_connections(result));
        });
    }

    pub fn load_process(self: Pin<&mut Self>, pid: i32) {
        if pid <= 0 {
            return;
        }
        let thread = self.qt_thread();
        std::thread::spawn(move || {
            let result = local_api::get(&format!("/v1/processes/{pid}"));
            let _ = thread.queue(move |machine| machine.apply_process_detail(result));
        });
    }

    pub fn refresh_software(mut self: Pin<&mut Self>) {
        if self.software_loading {
            return;
        }
        self.as_mut().set_software_loading(true);
        let thread = self.qt_thread();
        std::thread::spawn(move || {
            let result = local_api::get("/v1/software");
            let _ = thread.queue(move |machine| machine.apply_software(result));
        });
    }

    pub fn load_software(self: Pin<&mut Self>, package_type: &QString, identity: &QString) {
        let package_type = url_component(&package_type.to_string());
        let identity = url_component(&identity.to_string());
        let thread = self.qt_thread();
        std::thread::spawn(move || {
            let result = local_api::get(&format!(
                "/v1/software/detail?manager={package_type}&identity={identity}"
            ));
            let _ = thread.queue(move |machine| machine.apply_software_detail(result));
        });
    }

    pub fn trigger_scan(mut self: Pin<&mut Self>) {
        self.as_mut()
            .set_operation_message(QString::from("Scan requested"));
        let thread = self.qt_thread();
        std::thread::spawn(move || {
            let result = local_api::post(
                "/v1/actions/trigger-scan",
                &Value::Object(Default::default()),
            );
            let _ = thread.queue(move |machine| machine.apply_operation(result));
        });
    }

    // Standalone approval. The Agent runs dry-run, closure hash resolution, OSV,
    // mint, and helper execution; this only carries the request and renders the
    // verdict it gets back. No package-manager path exists in this process.
    pub fn approve_update(
        mut self: Pin<&mut Self>,
        package_type: &QString,
        package_name: &QString,
        available_version: &QString,
        override_reason: &QString,
    ) {
        if self.approval_running {
            return;
        }
        let request = serde_json::json!({
            "package_type": package_type.to_string(),
            "package_name": package_name.to_string(),
            "available_version": available_version.to_string(),
            // Asserted from the session that opened the socket, not attested.
            // ARCH-002 leaves fresh StepUp and real attribution unfinished.
            "operator": std::env::var("USER")
                .or_else(|_| std::env::var("USERNAME"))
                .unwrap_or_else(|_| "local".into()),
            "override_reason": override_reason.to_string(),
        });
        self.as_mut().set_approval_running(true);
        self.as_mut().set_approval_json(QString::from("{}"));
        let thread = self.qt_thread();
        std::thread::spawn(move || {
            let result = local_api::post("/v1/actions/approve-update", &request);
            let _ = thread.queue(move |machine| machine.apply_approval(result));
        });
    }

    fn apply_approval(mut self: Pin<&mut Self>, result: Result<Value, String>) {
        self.as_mut().set_approval_running(false);
        let success = result.is_ok();
        let payload = match result {
            Ok(value) => value,
            Err(error) => serde_json::json!({ "error": error }),
        };
        self.as_mut().set_approval_json(json_string(&payload));
        self.approval_finished(success);
    }

    fn apply_telemetry(mut self: Pin<&mut Self>, result: Result<Value, String>) {
        self.as_mut().set_telemetry_loading(false);
        match result {
            Ok(value) => {
                let cpu = &value["cpu"];
                let memory = &value["memory"];
                let network = &value["network"];
                let storage = &value["storage"];
                self.as_mut().set_cpu_usage(number(cpu, "usage_percent"));
                self.as_mut().set_load_1(number(cpu, "load_1"));
                self.as_mut().set_load_5(number(cpu, "load_5"));
                self.as_mut().set_load_15(number(cpu, "load_15"));
                self.as_mut()
                    .set_memory_percent(number(memory, "used_percent"));
                self.as_mut().set_memory_used(integer(memory, "used_bytes"));
                self.as_mut()
                    .set_memory_total(integer(memory, "total_bytes"));
                self.as_mut()
                    .set_swap_percent(number(memory, "swap_percent"));
                self.as_mut()
                    .set_network_receive(number(network, "receive_bytes_per_second"));
                self.as_mut()
                    .set_network_transmit(number(network, "transmit_bytes_per_second"));
                self.as_mut()
                    .set_disk_read(number(storage, "read_bytes_per_second"));
                self.as_mut()
                    .set_disk_write(number(storage, "write_bytes_per_second"));
                let hottest = value["thermals"]
                    .as_array()
                    .into_iter()
                    .flatten()
                    .filter_map(|reading| reading["celsius"].as_f64())
                    .fold(0.0_f64, f64::max);
                self.as_mut().set_temperature(hottest);
                self.as_mut()
                    .set_history_json(json_string(&value["history"]));
                self.as_mut().set_connected(true);
                self.as_mut().set_connection_error(QString::default());
                self.telemetry_updated();
            }
            Err(error) => self.as_mut().report_connection_error(&error),
        }
    }

    fn apply_overview(mut self: Pin<&mut Self>, result: Result<Overview, String>) {
        self.as_mut().set_overview_loading(false);
        match result {
            Ok(data) => self.as_mut().apply_overview_data(data),
            Err(error) => self.as_mut().report_connection_error(&error),
        }
    }

    fn apply_overview_data(mut self: Pin<&mut Self>, data: Overview) {
        let gap = if data.missing.is_empty() {
            String::new()
        } else {
            format!(
                "This Agent does not serve {}. Update the RedFlag Agent to fill these views — the machine is reachable, the Agent is behind.",
                data.missing.join(", ")
            )
        };
        self.as_mut().set_agent_gap(QString::from(gap.as_str()));
        let info = &data.system["system"];
        let name = text(&data.identity, "display_name")
            .or_else(|| text(info, "hostname"))
            .unwrap_or_else(|| "This machine".into());
        let os = text(info, "os_version")
            .or_else(|| text(&data.identity, "os_type"))
            .unwrap_or_else(|| "Unknown OS".into());
        let model = text(info, "device_model")
            .or_else(|| text(info, "device_type"))
            .unwrap_or_else(|| "Computer".into());
        let registered = data.identity["registered"].as_bool().unwrap_or(false);
        let updates = data.updates["updates"]
            .as_array()
            .cloned()
            .unwrap_or_default();
        let critical = data.security["critical_updates"].as_i64().unwrap_or(0) as i32;
        let memory = info["memory_info"]["used_percent"].as_f64().unwrap_or(0.0);
        let disk_pressure = info["disk_info"]
            .as_array()
            .into_iter()
            .flatten()
            .any(|disk| disk["used_percent"].as_f64().unwrap_or(0.0) >= 90.0);
        let failed_services = data.services["failed"].as_i64().unwrap_or(0) as i32;
        let unhealthy = data.containers["unhealthy"].as_i64().unwrap_or(0) as i32;
        let degraded = data.security["degraded_mode"].as_bool().unwrap_or(false);
        let health = if degraded
            || critical > 0
            || memory >= 90.0
            || disk_pressure
            || failed_services > 0
            || unhealthy > 0
        {
            "degraded"
        } else {
            "healthy"
        };

        self.as_mut().set_hostname(QString::from(name.as_str()));
        self.as_mut()
            .set_machine_subtitle(QString::from(format!("{model} · {os}").as_str()));
        self.as_mut()
            .set_agent_version(qtext(&data.identity, "agent_version"));
        self.as_mut()
            .set_agent_status(qtext(&data.status, "agent_status"));
        self.as_mut().set_enrollment(QString::from(if registered {
            "fleet enrolled"
        } else {
            "standalone"
        }));
        self.as_mut().set_uptime(qtext(info, "uptime"));
        self.as_mut()
            .set_process_count(info["running_processes"].as_i64().unwrap_or(0) as i32);
        self.as_mut().set_update_count(updates.len() as i32);
        self.as_mut().set_critical_count(critical);
        self.as_mut()
            .set_container_count(data.containers["count"].as_i64().unwrap_or(0) as i32);
        self.as_mut()
            .set_container_running(data.containers["running"].as_i64().unwrap_or(0) as i32);
        self.as_mut().set_container_unhealthy(unhealthy);
        self.as_mut()
            .set_service_count(data.services["count"].as_i64().unwrap_or(0) as i32);
        self.as_mut()
            .set_service_running(data.services["running"].as_i64().unwrap_or(0) as i32);
        self.as_mut().set_service_failed(failed_services);
        self.as_mut().set_health_state(QString::from(health));
        self.as_mut().set_system_json(json_string(info));
        self.as_mut()
            .set_updates_json(json_string(&Value::Array(updates)));
        self.as_mut().set_security_json(json_string(&data.security));
        self.as_mut()
            .set_containers_json(json_string(&data.containers["containers"]));
        self.as_mut()
            .set_services_json(json_string(&data.services["services"]));
        self.as_mut()
            .set_events_json(json_string(&data.events["events"]));
        self.as_mut().set_connected(true);
        self.as_mut().set_connection_error(QString::default());
        self.overview_updated();
    }

    fn apply_processes(mut self: Pin<&mut Self>, result: Result<Value, String>) {
        self.as_mut().set_processes_loading(false);
        match result {
            Ok(value) => {
                self.as_mut()
                    .set_processes_json(json_string(&value["processes"]));
                self.as_mut()
                    .set_process_count(value["process_count"].as_i64().unwrap_or(0) as i32);
                self.processes_updated();
            }
            Err(error) => self
                .as_mut()
                .set_operation_message(QString::from(error.as_str())),
        }
    }

    fn apply_connections(mut self: Pin<&mut Self>, result: Result<Value, String>) {
        match result {
            Ok(value) => {
                self.as_mut()
                    .set_connections_json(json_string(&value["connections"]));
                self.connections_updated();
            }
            Err(error) => self
                .as_mut()
                .set_operation_message(QString::from(error.as_str())),
        }
    }

    fn apply_process_detail(mut self: Pin<&mut Self>, result: Result<Value, String>) {
        match result {
            Ok(value) => self.as_mut().set_process_detail_json(json_string(&value)),
            Err(error) => self
                .as_mut()
                .set_process_detail_json(json_string(&serde_json::json!({"error": error}))),
        }
        self.process_detail_updated();
    }

    fn apply_software(mut self: Pin<&mut Self>, result: Result<Value, String>) {
        self.as_mut().set_software_loading(false);
        match result {
            Ok(value) => {
                self.as_mut()
                    .set_software_count(value["count"].as_i64().unwrap_or(0) as i32);
                self.as_mut().set_software_explicit_count(
                    value["explicit_count"].as_i64().unwrap_or(0) as i32,
                );
                self.as_mut().set_software_dependency_count(
                    value["dependency_count"].as_i64().unwrap_or(0) as i32,
                );
                self.as_mut().set_software_foreign_count(
                    value["foreign_count"].as_i64().unwrap_or(0) as i32,
                );
                self.as_mut().set_software_json(json_string(&value));
                self.software_updated();
            }
            Err(error) => self
                .as_mut()
                .set_operation_message(QString::from(error.as_str())),
        }
    }

    fn apply_software_detail(mut self: Pin<&mut Self>, result: Result<Value, String>) {
        let payload = match result {
            Ok(value) => value,
            Err(error) => serde_json::json!({ "error": error }),
        };
        self.as_mut()
            .set_software_detail_json(json_string(&payload));
        self.software_detail_updated();
    }

    fn apply_operation(mut self: Pin<&mut Self>, result: Result<Value, String>) {
        let (success, message) = match result {
            Ok(value) => (
                value["accepted"].as_bool().unwrap_or(true),
                "Agent scan started".to_string(),
            ),
            Err(error) => (false, error),
        };
        self.as_mut()
            .set_operation_message(QString::from(message.as_str()));
        self.operation_finished(success);
    }

    fn report_connection_error(mut self: Pin<&mut Self>, error: &str) {
        self.as_mut().set_connected(false);
        self.as_mut().set_agent_status(QString::from("unreachable"));
        self.as_mut().set_health_state(QString::from("unknown"));
        self.as_mut().set_connection_error(QString::from(error));
    }
}

struct Overview {
    identity: Value,
    status: Value,
    system: Value,
    updates: Value,
    security: Value,
    containers: Value,
    services: Value,
    events: Value,
    missing: Vec<&'static str>,
}

fn load_overview() -> Result<Overview, String> {
    // Presence is a report to the Agent, never a self-declared health verdict.
    // The Agent owns liveness and may surface or journal a missing desktop.
    let _ = local_api::post(
        "/v1/desktop",
        &serde_json::json!({
            "version": env!("REDFLAG_VERSION"),
            "uptime_seconds": DESKTOP_START.elapsed().as_secs(),
            "window_open": true
        }),
    );
    // identity and status are the contract every Agent has served. Everything
    // below them arrived later, so a 404 names an Agent that predates the
    // endpoint rather than a machine that cannot answer.
    let identity = local_api::get("/v1/identity")?;
    let status = local_api::get("/v1/status")?;
    let mut missing = Vec::new();
    let mut optional =
        |path: &str, name: &'static str, empty: Value| match local_api::get_optional(path) {
            Ok(Some(value)) => value,
            Ok(None) => {
                missing.push(name);
                empty
            }
            Err(_) => empty,
        };
    Ok(Overview {
        system: optional("/v1/system", "machine health", empty_object()),
        updates: optional("/v1/packages", "packages", empty_list("updates")),
        security: optional("/v1/security", "security posture", empty_object()),
        containers: optional("/v1/containers", "containers", empty_list("containers")),
        services: optional("/v1/services", "services", empty_list("services")),
        events: optional("/v1/events", "history", empty_list("events")),
        missing,
        identity,
        status,
    })
}

fn empty_object() -> Value {
    Value::Object(serde_json::Map::new())
}

fn empty_list(key: &str) -> Value {
    let mut object = serde_json::Map::new();
    object.insert(key.to_string(), Value::Array(Vec::new()));
    Value::Object(object)
}

fn number(value: &Value, key: &str) -> f64 {
    value[key].as_f64().unwrap_or(0.0)
}

fn integer(value: &Value, key: &str) -> u64 {
    value[key].as_u64().unwrap_or(0)
}

fn text(value: &Value, key: &str) -> Option<String> {
    value[key]
        .as_str()
        .filter(|text| !text.is_empty())
        .map(str::to_owned)
}

fn qtext(value: &Value, key: &str) -> QString {
    QString::from(text(value, key).unwrap_or_default().as_str())
}

fn json_string(value: &Value) -> QString {
    QString::from(
        serde_json::to_string(value)
            .unwrap_or_else(|_| "null".into())
            .as_str(),
    )
}

fn url_component(value: &str) -> String {
    let mut encoded = String::with_capacity(value.len());
    for byte in value.bytes() {
        if byte.is_ascii_alphanumeric() || matches!(byte, b'-' | b'_' | b'.' | b'~') {
            encoded.push(byte as char);
        } else {
            use std::fmt::Write;
            let _ = write!(&mut encoded, "%{byte:02X}");
        }
    }
    encoded
}
