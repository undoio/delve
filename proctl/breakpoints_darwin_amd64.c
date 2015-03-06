#include "breakpoints_darwin_amd64.h"
#include <stdio.h>

uint64_t
get_control_register(thread_act_t task) {
	kern_return_t kret;
	struct __darwin_x86_debug_state64 state;
	mach_msg_type_number_t stateCount = X86_DEBUG_STATE64_COUNT;

	kret = thread_get_state(task, x86_DEBUG_STATE64, (thread_state_t)&state, &stateCount);
	if (kret != KERN_SUCCESS) {
		errno = EINVAL;
		return 0;
	}

	return state.__dr7;
}

kern_return_t
set_control_register(thread_act_t task, uint64_t dr7) {
	kern_return_t kret;
	struct __darwin_x86_debug_state64 state;
	mach_msg_type_number_t stateCount = X86_DEBUG_STATE64_COUNT;

	kret = thread_get_state(task, x86_DEBUG_STATE64, (thread_state_t)&state, &stateCount);
	if (kret != KERN_SUCCESS) return kret;

	state.__dr7 = dr7;

	return thread_set_state(task, x86_DEBUG_STATE64, (thread_state_t)&state, stateCount);
}

kern_return_t
set_debug_register(thread_act_t task, int reg, uint64_t drx) {
	kern_return_t kret;
	struct __darwin_x86_debug_state64 state;
	mach_msg_type_number_t stateCount = X86_DEBUG_STATE64_COUNT;

	kret = thread_get_state(task, x86_DEBUG_STATE64, (thread_state_t)&state, &stateCount);
	if (kret != KERN_SUCCESS) return kret;

	switch(reg) {
	case 0:
		state.__dr0 = drx;
		break;
	case 1:
		state.__dr1 = drx;
		break;
	case 2:
		state.__dr2 = drx;
		break;
	case 3:
		state.__dr3 = drx;
		break;
	}

	return thread_set_state(task, x86_DEBUG_STATE64, (thread_state_t)&state, stateCount);
}
