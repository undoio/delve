#include "memory_darwin.h"

int
write_memory(task_t task, mach_vm_address_t addr, void *d, mach_msg_type_number_t len) {
	kern_return_t kret;
	vm_region_submap_short_info_data_64_t info;
	mach_msg_type_number_t count = VM_REGION_SUBMAP_SHORT_INFO_COUNT_64;
	mach_vm_size_t l = len;
	mach_port_t objname;

	if (len == 1) l = 2;
	kret = mach_vm_region((vm_map_t)task, &(mach_vm_address_t){addr}, (mach_vm_size_t*)&l, VM_REGION_BASIC_INFO_64, (vm_region_info_t)&info, &count, &objname);
	if (kret != KERN_SUCCESS) return -1;

	// Set permissions to enable writting to this memory location
	kret = mach_vm_protect(task, addr, len, FALSE, VM_PROT_WRITE|VM_PROT_COPY|VM_PROT_READ);
	if (kret != KERN_SUCCESS) return -1;

	kret = mach_vm_write((vm_map_t)task, addr, (vm_offset_t)d, len);
	if (kret != KERN_SUCCESS) return -1;

	// Restore virtual memory permissions
	kret = mach_vm_protect(task, addr, len, FALSE, info.protection);
	if (kret != KERN_SUCCESS) return -1;

	return 0;
}

int
read_memory(task_t task, mach_vm_address_t addr, void *d, mach_msg_type_number_t len) {
	kern_return_t kret;
	pointer_t data;
	mach_msg_type_number_t count;

	kret = mach_vm_read((vm_map_t)task, addr, len, &data, &count);
	if (kret != KERN_SUCCESS) return -1;
	memcpy(d, (void *)data, len);

	kret = vm_deallocate(task, data, len);
	if (kret != KERN_SUCCESS) return -1;

	return count;
}

