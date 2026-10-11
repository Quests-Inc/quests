// Package kernel is the shared kernel of the contexts (ADR-0001): a few small rules several of them must apply the
// same way — a runner label, an icon file name, who may write a command. It depends on identity and platform only;
// journal, knowledge and automation may import it.
package kernel
