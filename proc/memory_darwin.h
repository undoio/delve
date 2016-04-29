#include <stdlib.h>
#include <sys/types.h>
#include <mach/mach.h>
#include <mach/mach_vm.h>

int
write_memory(task_t, mach_vm_address_t, void *, mach_msg_type_number_t);

int
read_memory(task_t, mach_vm_address_t, void *, mach_msg_type_number_t);

