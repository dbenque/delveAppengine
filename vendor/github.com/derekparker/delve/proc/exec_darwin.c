#include "exec_darwin.h"

extern char** environ;

int
close_exec_pipe(int fd[2]) {
	if (pipe(fd) < 0) return -1;
	if (fcntl(fd[0], F_SETFD, FD_CLOEXEC) < 0) return -1;
	if (fcntl(fd[1], F_SETFD, FD_CLOEXEC) < 0) return -1;
	return 0;
}

int
fork_exec(char *argv0, char **argv, int size,
		task_t *task,
		mach_port_t *port_set,
		mach_port_t *exception_port,
		mach_port_t *notification_port)
{
	// Since we're using mach exceptions instead of signals,
	// we need to coordinate between parent and child via pipes
	// to ensure that the parent has set the exception ports on
	// the child task before it execs.
	int fd[2];
	if (close_exec_pipe(fd) < 0) return -1;

	// Create another pipe to signal the parent on exec.
	int efd[2];
	if (close_exec_pipe(efd) < 0) return -1;

	kern_return_t kret;
	pid_t pid = fork();
	if (pid > 0) {
		// In parent.
		close(fd[0]);
		close(efd[1]);
		kret = acquire_mach_task(pid, task, port_set, exception_port, notification_port);
		if (kret != KERN_SUCCESS) return -1;

		char msg = 'c';
		write(fd[1], &msg, 1);
		close(fd[1]);

		char w;
		size_t n = read(efd[0], &w, 1);
		close(efd[0]);
		if (n != 0) {
			// Child died, reap it.
			waitpid(pid, NULL, 0);
			return -1;
		}
		return pid;
	}

	// Fork succeeded, we are in the child.
	int pret;
	char sig;

	close(fd[1]);
	read(fd[0], &sig, 1);
	close(fd[0]);

	// Create a new process group.
	if (setpgid(0, 0) < 0) {
		return -1;
	}

	// Set errno to zero before a call to ptrace.
	// It is documented that ptrace can return -1 even
	// for successful calls.
	errno = 0;
	pret = ptrace(PT_TRACE_ME, 0, 0, 0);
	if (pret != 0 && errno != 0) return -errno;

	errno = 0;
	pret = ptrace(PT_SIGEXC, 0, 0, 0);
	if (pret != 0 && errno != 0) return -errno;

	// Create the child process.
	execve(argv0, argv, environ);

	// We should never reach here, but if we did something went wrong.
	// Write a message to parent to alert that exec failed.
	char msg = 'd';
	write(efd[1], &msg, 1);
	close(efd[1]);

	exit(1);
}
