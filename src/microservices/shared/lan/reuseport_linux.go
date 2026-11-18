package lan

// soReusePort — SO_REUSEPORT sous Linux et Android : le paquet syscall ne le
// nomme pas sur toutes les architectures.
const soReusePort = 0xf
