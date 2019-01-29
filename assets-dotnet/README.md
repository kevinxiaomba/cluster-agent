# Welcome to AppDynamics .NET agent for .NET core on Linux

Make sure to always download the latest agent version.
For details and documentation, visit: https://docs.appdynamics.com/

## Supported OS

- Ubuntu 14+
- CentOS 7+
- Alpine 3.7+
- Debian 8+

## Supported .NET Core Version

- .NET core 2.0+

## Setup

- Place AppDynamics.agent.netstandard.dll and libappdprofiler.so and AppDynamicsConfig.json in the same folder
- Update AppDynamicsConfig.json or set environment variables with controller details
- Set these environment variables to enable the agent:

```sh
CORECLR_PROFILER={57e1aa68-2229-41aa-9931-a6e93bbc64d8}
CORECLR_ENABLE_PROFILING=1
CORECLR_PROFILER_PATH=<full_path_to_libappdprofiler.so>
```

- For Alpine distribution, set additional environment variable:
```sh
LD_LIBRARY_PATH=<full_path_to_agent_folder>
```

## Sample Dockerfile for CentOS/Ubuntu/Debian/RHEL

```sh
# Copy agent binaries to the image from current folder
RUN mkdir -p /opt/appdynamics/dotnet
ADD libappdprofiler.so /opt/appdynamics/dotnet/
ADD AppDynamics.Agent.netstandard.dll /opt/appdynamics/dotnet/

# Mandatory settings required to attach the agent to the .NET application
ENV CORECLR_PROFILER={57e1aa68-2229-41aa-9931-a6e93bbc64d8} \
    CORECLR_ENABLE_PROFILING=1 \
    CORECLR_PROFILER_PATH=/opt/appdynamics/dotnet/libappdprofiler.so

# Configure connection to the controller
ENV APPDYNAMICS_CONTROLLER_HOST_NAME=controller.saas.appdynamics.com
ENV APPDYNAMICS_CONTROLLER_PORT=443
ENV APPDYNAMICS_CONTROLLER_SSL_ENABLED=true
ENV APPDYNAMICS_AGENT_ACCOUNT_NAME=
ENV APPDYNAMICS_AGENT_ACCOUNT_ACCESS_KEY=

# Configure application identity in AppDynamics
ENV APPDYNAMICS_AGENT_APPLICATION_NAME=
ENV APPDYNAMICS_AGENT_TIER_NAME=
ENV APPDYNAMICS_AGENT_NODE_NAME=
```

## Sample Dockerfile for Alpine

```sh
# Copy agent binaries to the image from current folder
RUN mkdir -p /opt/appdynamics/dotnet
ADD libappdprofiler.so /opt/appdynamics/dotnet/
ADD AppDynamics.Agent.netstandard.dll /opt/appdynamics/dotnet/

# Mandatory settings required to attach the agent to the .NET application
ENV CORECLR_PROFILER={57e1aa68-2229-41aa-9931-a6e93bbc64d8} \
    CORECLR_ENABLE_PROFILING=1 \
    CORECLR_PROFILER_PATH=/opt/appdynamics/dotnet/libappdprofiler.so \
    LD_LIBRARY_PATH=/opt/appdynamics/dotnet

# Configure connection to the controller
ENV APPDYNAMICS_CONTROLLER_HOST_NAME=controller.saas.appdynamics.com
ENV APPDYNAMICS_CONTROLLER_PORT=443
ENV APPDYNAMICS_CONTROLLER_SSL_ENABLED=true
ENV APPDYNAMICS_AGENT_ACCOUNT_NAME=
ENV APPDYNAMICS_AGENT_ACCOUNT_ACCESS_KEY=

# Configure application identity in AppDynamics
ENV APPDYNAMICS_AGENT_APPLICATION_NAME=
ENV APPDYNAMICS_AGENT_TIER_NAME=
ENV APPDYNAMICS_AGENT_NODE_NAME=
```