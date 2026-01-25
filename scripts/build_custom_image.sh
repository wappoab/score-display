#!/bin/bash
set -e

# --- STATIC CONFIGURATION ---
INPUT_IMAGE="image/2025-12-04-raspios-trixie-arm64.img.xz"
OUTPUT_IMAGE="image/display-client-custom.img"
MOUNT_ROOT="/mnt/rpi_root"
MOUNT_BOOT="/mnt/rpi_boot"

# Client Files
CLIENT_BINARY="bin/client-arm64"
STATIC_DIR="client/static"
PI_USER="pi"
# ----------------------------

echo "=== Raspberry Pi Image Customizer (Robust Mode) ==="

if [ "$EUID" -ne 0 ]; then
  echo "Please run as root (sudo $0)"
  exit 1
fi

# 1. Check Files
if [ ! -f "$INPUT_IMAGE" ] && [ ! -f "$OUTPUT_IMAGE" ]; then
  echo "Error: Input image $INPUT_IMAGE not found."
  exit 1
fi

if [ ! -f "$CLIENT_BINARY" ]; then
  echo "Error: Client binary $CLIENT_BINARY not found."
  exit 1
fi

# 2. Collect User Input
echo ""
echo "--- Configuration Required ---"
read -p "Enter WiFi SSID: " WIFI_SSID
read -s -p "Enter WiFi Password: " WIFI_PASS
echo ""
read -s -p "Enter Password for user '$PI_USER': " PI_PASS
echo ""
echo "------------------------------"
echo ""

# 3. Prepare Image
if [ ! -f "$OUTPUT_IMAGE" ]; then
    echo "Decompressing base image (this takes a while)..."
    xz -d -k -c "$INPUT_IMAGE" > "$OUTPUT_IMAGE"
else
    echo "Using existing image: $OUTPUT_IMAGE"
fi

# 4. Attach Loop Device with Partition Scan
echo "Attaching image to loop device..."
LOOP_DEV=$(losetup -fP --show "$OUTPUT_IMAGE")
echo "Image attached to $LOOP_DEV"

cleanup() {
    echo "Unmounting and detaching..."
    umount "$MOUNT_BOOT" || true
    umount "$MOUNT_ROOT" || true
    rmdir "$MOUNT_BOOT" "$MOUNT_ROOT" || true
    losetup -d "$LOOP_DEV" || true
}
trap cleanup EXIT

# Wait a moment for partitions to appear
sleep 1

# 5. Mount Partitions
echo "Mounting partitions..."
mkdir -p "$MOUNT_BOOT" "$MOUNT_ROOT"
mount "${LOOP_DEV}p1" "$MOUNT_BOOT"
mount "${LOOP_DEV}p2" "$MOUNT_ROOT"

# 6. Configure User (Skip First Boot Wizard)
echo "Configuring user '$PI_USER'..."
# Create userconf.txt in BOOT partition
ENCRYPTED_PASS=$(echo -n "$PI_PASS" | openssl passwd -6 -stdin)
echo "$PI_USER:$ENCRYPTED_PASS" > "$MOUNT_BOOT/userconf.txt"
touch "$MOUNT_BOOT/ssh" # Enable SSH

# 7. Configure WiFi (NetworkManager)
echo "Configuring WiFi for SSID: $WIFI_SSID..."
NM_DIR="$MOUNT_ROOT/etc/NetworkManager/system-connections"
mkdir -p "$NM_DIR"
cat > "$NM_DIR/preconfigured.nmconnection" <<EOF
[connection]
id=preconfigured
type=wifi

[wifi]
mode=infrastructure
ssid=$WIFI_SSID

[wifi-security]
auth-alg=open
key-mgmt=wpa-psk
psk=$WIFI_PASS

[ipv4]
method=auto

[ipv6]
method=auto
EOF
chmod 600 "$NM_DIR/preconfigured.nmconnection"

# 8. Enable Auto-login
echo "Enabling Auto-login..."
# For lightdm (X11)
if [ -f "$MOUNT_ROOT/etc/lightdm/lightdm.conf" ]; then
    sed -i "s/^#autologin-user=.*/autologin-user=$PI_USER/" "$MOUNT_ROOT/etc/lightdm/lightdm.conf"
fi

# 9. Install Display Client
echo "Installing Display Client..."
TARGET_DIR="$MOUNT_ROOT/home/pi/display-client"
mkdir -p "$TARGET_DIR/static"
cp "$CLIENT_BINARY" "$TARGET_DIR/client"
cp -r "$STATIC_DIR/"* "$TARGET_DIR/static/"
chmod +x "$TARGET_DIR/client"

# 10. Configure Autostart & Power Management
echo "Configuring Autostart & Power..."
AUTOSTART_DIR="$MOUNT_ROOT/home/pi/.config/autostart"
mkdir -p "$AUTOSTART_DIR"
cat > "$AUTOSTART_DIR/display.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=Display Client
Exec=/home/pi/display-client/client -kiosk
X-GNOME-Autostart-enabled=true
EOF

# Disable Blanking (Wayland/Labwc)
LABWC_DIR="$MOUNT_ROOT/home/pi/.config/labwc"
mkdir -p "$LABWC_DIR"
echo "wlr-randr --output HDMI-A-1 --on" >> "$LABWC_DIR/autostart"

# 11. Fix Permissions
echo "Fixing file permissions..."
chown -R 1000:1000 "$MOUNT_ROOT/home/pi"

echo "=== Success ==="
echo "The image is ready: $OUTPUT_IMAGE"
echo "Write it to your SD card using 'dd'."
