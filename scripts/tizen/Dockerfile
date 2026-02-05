FROM ubuntu:20.04

ENV DEBIAN_FRONTEND=noninteractive
ENV TZ=UTC

RUN apt-get update && apt-get install -y \
    wget \
    zip \
    unzip \
    libncurses5 \
    default-jdk \
    pciutils \
    && rm -rf /var/lib/apt/lists/*

# Install Tizen Studio CLI
# URL from standard Tizen Studio update site
ENV TIZEN_VERSION=4.5
ENV TIZEN_CLI_URL=http://download.tizen.org/sdk/Installer/tizen-studio_4.5/web-cli_Tizen_Studio_4.5_ubuntu-64.bin

WORKDIR /tmp
RUN wget -q ${TIZEN_CLI_URL} -O tizen-web-cli.bin \
    && chmod +x tizen-web-cli.bin \
    && ./tizen-web-cli.bin --accept-license /tizen-studio \
    && rm tizen-web-cli.bin

ENV PATH="/tizen-studio/tools/ide/bin:/tizen-studio/tools:${PATH}"

# Install Web Certs
# We need to run package-manager inside... this is hard in docker non-interactive.
# Alternatively, we can just use the 'tizen' command which is now installed.

WORKDIR /workspace
CMD ["/bin/bash"]

