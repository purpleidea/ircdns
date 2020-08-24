// IRCdns
// Copyright (C) 2019-2020+ James Shubin and the project contributors
// Written by James Shubin <james@shubin.ca> and the project contributors
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

// The boilerplate for this file was taken from Leo Antunes <leo@costela.net>
// and was listed as Copyright 2018, GPLv3+.

// Builds an nss .so file for use with /etc/nsswitch.conf
package main

// #include <stdlib.h>
// #include <errno.h>
// #include <nss.h>
// #include <netdb.h>
// #include <arpa/inet.h>
import "C"
import (
	"runtime"
	"strings"
	"unsafe"
)

func init() {
	runtime.GOMAXPROCS(1) // we don't need extra goroutines
}

//export _nss_ircdns_gethostbyname3_r
func _nss_ircdns_gethostbyname3_r(name *C.char, af C.int, result *C.struct_hostent,
	buffer *C.char, buflen C.size_t, errnop *C.int, herrnop *C.int, ttlp *C.int32_t,
	canonp **C.char) C.enum_nss_status {

	if af == C.AF_UNSPEC {
		af = C.AF_INET
	}

	if af != C.AF_INET {
		return unavailable(errnop, herrnop)
	}

	queryName := C.GoString(name)

	// If configured, look for our ".ircdns" suffix if we specified one.
	if len(queryName) == 0 || !strings.HasSuffix(queryName, Suffix) {
		return unavailable(errnop, herrnop)
	}

	// This is where we get the mapping!
	addresses, err := Lookup(queryName)
	if err != nil {
		return unavailable(errnop, herrnop)
	}

	if len(addresses) == 0 {
		return notfound(errnop, herrnop)
	}

	// buffer must fit addresses and respective pointers + 1 (NULL pointer)
	cAddressesSize := C.size_t(len(addresses)) * C.sizeof_struct_in_addr
	cAddressPtrsSize := uintptr(len(addresses)+1) * unsafe.Sizeof(uintptr(0))
	if buflen < (cAddressesSize + C.size_t(cAddressPtrsSize)) {
		return bufferTooSmall(errnop, herrnop)
	}

	// TODO: is there really no cleaner way to access the data as an array?
	cAddressPtrs := (*[1 << 30]*C.char)(unsafe.Pointer(buffer))
	cAddresses := (*[1 << 30]*C.char)(unsafe.Pointer(uintptr(unsafe.Pointer(buffer)) + cAddressPtrsSize))
	for i, a := range addresses {
		cAddressPtrs[i] = (*C.char)(unsafe.Pointer(&cAddresses[i]))
		if ret := C.inet_aton(C.CString(a), (*C.struct_in_addr)(unsafe.Pointer(&cAddresses[i]))); ret != C.int(1) {
			return unavailable(errnop, herrnop)
		}
	}
	cAddressPtrs[len(addresses)] = nil

	result.h_name = name
	result.h_aliases = (**C.char)(unsafe.Pointer(&cAddressPtrs[len(addresses)])) // TODO: actually build alias-list
	result.h_addrtype = af
	result.h_length = C.sizeof_struct_in_addr
	result.h_addr_list = (**C.char)(unsafe.Pointer(buffer))

	return C.NSS_STATUS_SUCCESS
}

//export _nss_ircdns_gethostbyname2_r
func _nss_ircdns_gethostbyname2_r(name *C.char, af C.int, result *C.struct_hostent,
	buffer *C.char, buflen C.size_t, errnop *C.int, herrnop *C.int) C.enum_nss_status {
	return _nss_ircdns_gethostbyname3_r(name, af, result, buffer, buflen, errnop, herrnop, nil, nil)
}

//export _nss_ircdns_gethostbyname_r
func _nss_ircdns_gethostbyname_r(name *C.char, result *C.struct_hostent, buffer *C.char,
	buflen C.size_t, errnop *C.int, herrnop *C.int) C.enum_nss_status {
	return _nss_ircdns_gethostbyname3_r(name, C.AF_UNSPEC, result, buffer, buflen, errnop, herrnop, nil, nil)
}

func unavailable(errnop, herrnop *C.int) C.enum_nss_status {
	*errnop = C.ENOENT
	*herrnop = C.NO_DATA
	return C.NSS_STATUS_UNAVAIL
}

func retry(errnop, herrnop *C.int) C.enum_nss_status {
	*errnop = C.EAGAIN
	*herrnop = C.NO_RECOVERY
	return C.NSS_STATUS_TRYAGAIN
}

func bufferTooSmall(errnop, herrnop *C.int) C.enum_nss_status {
	*errnop = C.ERANGE
	*herrnop = C.NETDB_INTERNAL
	return C.NSS_STATUS_TRYAGAIN
}

func notfound(errnop *C.int, herrnop *C.int) C.enum_nss_status {
	*errnop = C.ENOENT
	*herrnop = C.HOST_NOT_FOUND
	return C.NSS_STATUS_NOTFOUND
}

func main() {}
