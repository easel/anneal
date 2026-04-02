package engine

// stdlibPreamble defines shell functions that plan scripts call.
// These are prepended to every script before execution.
const stdlibPreamble = `
stdlib_file_write() {
  _path="$1"; _mode="$2"; _owner="$3"
  _content="$(cat)"
  _dir="$(dirname "$_path")"
  [ -d "$_dir" ] || mkdir -p "$_dir"
  printf '%s' "$_content" > "$_path"
  chmod "$_mode" "$_path"
  chown "$_owner" "$_path" 2>/dev/null || true
}

stdlib_file_copy() {
  _src="$1"; _dest="$2"; _mode="$3"; _owner="$4"
  _dir="$(dirname "$_dest")"
  [ -d "$_dir" ] || mkdir -p "$_dir"
  cp "$_src" "$_dest"
  chmod "$_mode" "$_dest"
  chown "$_owner" "$_dest" 2>/dev/null || true
}

stdlib_dir_create() {
  _path="$1"; _mode="$2"; _owner="$3"
  mkdir -p "$_path"
  chmod "$_mode" "$_path"
  chown "$_owner" "$_path" 2>/dev/null || true
}

stdlib_symlink() {
  _target="$1"; _link="$2"
  _dir="$(dirname "$_link")"
  [ -d "$_dir" ] || mkdir -p "$_dir"
  ln -snf "$_target" "$_link"
}

stdlib_file_remove() {
  _path="$1"
  rm -f "$_path"
}

stdlib_apt_install() {
  DEBIAN_FRONTEND=noninteractive apt-get install -y "$@"
}

stdlib_apt_purge() {
  DEBIAN_FRONTEND=noninteractive apt-get purge -y "$@"
}

stdlib_debconf_set() {
  echo "$@" | debconf-set-selections
}

stdlib_apt_key_add() {
  _url="$1"; _keyring="$2"
  _dir="$(dirname "$_keyring")"
  [ -d "$_dir" ] || mkdir -p "$_dir"
  curl -fsSL "$_url" | gpg --dearmor -o "$_keyring"
}

stdlib_apt_source_add() {
  _file="$1"; _line="$2"
  printf '%s\n' "$_line" > "$_file"
}

stdlib_deb_install() {
  _url="$1"; _tmp="$(mktemp /tmp/anneal-deb-XXXXXX.deb)"
  curl -fsSL -o "$_tmp" "$_url"
  DEBIAN_FRONTEND=noninteractive dpkg -i "$_tmp" || DEBIAN_FRONTEND=noninteractive apt-get install -f -y
  rm -f "$_tmp"
}

stdlib_brew_install() {
  _user="$1"; shift
  sudo -u "$_user" brew install "$@"
}

stdlib_brew_tap() {
  _user="$1"; _tap="$2"
  sudo -u "$_user" brew tap "$_tap"
}
`
