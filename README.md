# fanctl

`fanctl` is an advanced, lightweight Linux fan control system split into a background daemon (`fanctld`) and an interactive terminal UI client (`fanctl`). It allows precise, curve-based thermal management using standard `hwmon` interfaces, supporting temperature sensor aggregation (max/avg), automated PWM channel discovery, and live calibration.

## Features

* **Client-Server Architecture:** 
  * `fanctld`: Background daemon enforcing fan curves, handling polling, and exposing a local REST API.
  * `fanctl`: Interactive TUI wizard (powered by Bubble Tea) for real-time telemetry monitoring, auto-detection, and configuration.
* **Smart Temperature Aggregation:** Combine multiple hardware temperature sensors (CPU, NVMe, GPU) using `max` or `avg` logic to drive a single fan channel.
* **Automated PWM Calibration:** Built-in wizard that tests unassigned PWM channels against target fans by measuring actual RPM drops.
* **Flexible Fan Curves:** Define custom multi-point temperature-to-PWM curves with adjustable hysteresis, minimum, and maximum limits.
* **Systemd & Config Integration:** Clean integration with systemd (`fanctld.service`) and standard configuration paths (`/etc/fanctl/fanctl.conf`).

## Requirements

* Linux kernel with `hwmon` support (sysfs)
* Go 1.21 or higher (if building from source)

## Installation

### From Debian Package (`.deb`)
If you have built the `.deb` package using `dpkg-buildpackage`:
```bash
sudo apt install ./fanctl_0.14.0-1_amd64.deb
