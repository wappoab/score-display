#!/bin/bash
set -e

IMAGE_NAME="display-tizen-builder"
DOCKER_DIR="scripts/tizen"
PROJECT_ROOT=$(pwd)

echo "=== Tizen Builder (Docker) ==="

# 1. Build Docker Image if missing
if [[ "$(docker images -q $IMAGE_NAME 2> /dev/null)" == "" ]]; then
    echo "Building Docker image (this happens once)..."
    docker build -t $IMAGE_NAME $DOCKER_DIR
fi

# 2. Prepare Certificates
# We need a security profile. We'll generate a self-signed one inside.
# If you have real certs, put them in 'certs/' folder.

echo "Running Build in Docker..."
docker run --rm -v "$PROJECT_ROOT:/workspace" $IMAGE_NAME bash -c "
    set -e
    
    # Create Certificate if missing
    if [ ! -f /workspace/tizen-security-profile.xml ]; then
        echo 'Generating Self-Signed Certificate...'
        tizen certificate -a -n DisplayCert -p password -c US -s California -ct LosAngeles -o TizenUser -u TizenUser -e email@example.com -f /workspace/certs/display-cert
        
        # Add to profile
        tizen security-profiles add -n DisplayProfile -a /workspace/certs/display-cert.p12 -p password
    else
        # Re-import profile (tricky in ephemeral container, easier to regenerate or pass xml)
        # Actually, tizen CLI stores profiles in ~/tizen-studio-data.
        # So we MUST regenerate every time or mount that data.
        
        echo 'Re-generating Certificate for session...'
        mkdir -p /workspace/certs
        tizen certificate -a -n DisplayCert -p password -c US -s California -ct LosAngeles -o TizenUser -u TizenUser -e email@example.com -f /workspace/certs/display-cert --overwrite
        tizen security-profiles add -n DisplayProfile -a /workspace/certs/display-cert.p12 -p password
    fi

    # Build and Sign
    echo 'Packaging and Signing...'
    tizen package -t wgt -s DisplayProfile -- /workspace/client-tizen
    
    # Move to bin
    mkdir -p /workspace/bin
    mv /workspace/client-tizen/client-tizen.wgt /workspace/bin/client-tizen-signed.wgt
    echo 'Success! Signed package: bin/client-tizen-signed.wgt'
"
