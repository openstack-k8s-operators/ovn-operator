/*
Copyright 2020 Red Hat

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package util

import (
	corev1 "k8s.io/api/core/v1"
)

// Update a list of corev1.EnvVar in place

type EnvSetter func(*corev1.EnvVar)
type EnvSetterMap map[string]EnvSetter

func MergeEnvs(envs []corev1.EnvVar, newEnvs EnvSetterMap) []corev1.EnvVar {
	for name, f := range newEnvs {
		updated := false
		for i := 0; i < len(envs); i++ {
			if envs[i].Name == name {
				f(&envs[i])
				updated = true
				break
			}
		}

		if !updated {
			envs = append(envs, corev1.EnvVar{Name: name})
			f(&envs[len(envs)-1])
		}
	}

	return envs
}

func EnvDownwardAPI(field string) EnvSetter {
	return func(env *corev1.EnvVar) {
		if env.ValueFrom == nil {
			env.ValueFrom = &corev1.EnvVarSource{}
		}
		env.Value = ""

		if env.ValueFrom.FieldRef == nil {
			env.ValueFrom.FieldRef = &corev1.ObjectFieldSelector{}
		}

		env.ValueFrom.FieldRef.FieldPath = field
	}
}

func EnvValue(value string) EnvSetter {
	return func(env *corev1.EnvVar) {
		env.Value = value
		env.ValueFrom = nil
	}
}

// Update a list of corev1.VolumeMount in place

type MountSetter func(*corev1.VolumeMount)
type MountSetterMap map[string]MountSetter

func MergeVolumeMounts(mounts []corev1.VolumeMount, newMounts MountSetterMap) []corev1.VolumeMount {
	for name, f := range newMounts {
		updated := false
		for i := 0; i < len(mounts); i++ {
			if mounts[i].Name == name {
				f(&mounts[i])
				updated = true
				break
			}
		}

		if !updated {
			mounts = append(mounts, corev1.VolumeMount{Name: name})
			f(&mounts[len(mounts)-1])
		}
	}

	return mounts
}

func VolumeMount(mountPath string) MountSetter {
	return func(mount *corev1.VolumeMount) {
		mount.MountPath = mountPath
	}
}

func VolumeMountWithSubpath(mountPath, subPath string) MountSetter {
	return func(mount *corev1.VolumeMount) {
		mount.MountPath = mountPath
		mount.SubPath = subPath
	}
}

// Inititialise a label map to an empty map if it is nil.
func InitLabelMap(m *map[string]string) {
	if *m == nil {
		*m = make(map[string]string)
	}
}

// Syntactic sugar variables and functions

func ExecProbe(command ...string) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: command}},
	}
}

func EmptyDirVol() corev1.VolumeSource {
	return corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}
}
