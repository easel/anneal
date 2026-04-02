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
`
