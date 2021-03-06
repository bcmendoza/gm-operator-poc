package reconcilers

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/bcmendoza/gm-operator/pkg/api/v1"
	"github.com/bcmendoza/gm-operator/pkg/gmcore"
)

type Deployment struct {
	GmService gmcore.Service
	ObjectKey types.NamespacedName
}

func (d Deployment) Kind() string {
	return "appsv1.Deployment"
}

func (d Deployment) Key() types.NamespacedName {
	return d.ObjectKey
}

func (d Deployment) Object() client.Object {
	return &appsv1.Deployment{}
}

func (d Deployment) Reconcile(mesh *v1.Mesh, configs gmcore.Configs, obj client.Object) (client.Object, bool) {
	svc := d.GmService
	svcCfg := configs[svc]
	proxyCfg := configs[gmcore.Proxy]

	matchLabels := map[string]string{
		"greymatter.io/control": d.ObjectKey.Name,
	}

	podLabels := map[string]string{
		"greymatter.io/control":         d.ObjectKey.Name,
		"greymatter.io/component":       svcCfg.Component,
		"greymatter.io/service-version": svcCfg.ImageTag,
	}
	if svc != gmcore.Control {
		podLabels["greymatter.io/proxy-version"] = proxyCfg.ImageTag
	}

	objectLabels := map[string]string{
		"app.kubernetes.io/name":           d.ObjectKey.Name,
		"app.kubernetes.io/version":        svcCfg.ImageTag,
		"app.kubernetes.io/part-of":        "greymatter",
		"app.kubernetes.io/managed-by":     "gm-operator",
		"app.kubernetes.io/created-by":     "gm-operator",
		"greymatter.io/greymatter-version": mesh.Spec.Version,
	}
	for k, v := range podLabels {
		objectLabels[k] = v
	}

	svcContainer := corev1.Container{
		Name:            "service",
		Image:           fmt.Sprintf("docker.greymatter.io/%s/gm-%s:%s", svcCfg.Directory, svc, svcCfg.ImageTag),
		Env:             svcCfg.Envs.Apply(mesh, d.ObjectKey.Name),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports:           svcCfg.ContainerPorts,
	}
	if svcCfg.Resources != nil {
		svcContainer.Resources = *svcCfg.Resources
	}
	if svcCfg.VolumeMounts != nil {
		svcContainer.VolumeMounts = svcCfg.VolumeMounts
	}

	var containers []corev1.Container

	if d.ObjectKey.Name != "edge" {
		containers = append(containers, svcContainer)
	}

	if svc != gmcore.Control {
		proxyContainer := corev1.Container{
			Name:            "sidecar",
			Image:           fmt.Sprintf("docker.greymatter.io/%s/gm-proxy:%s", proxyCfg.Directory, proxyCfg.ImageTag),
			Env:             proxyCfg.Envs.Apply(mesh, d.ObjectKey.Name),
			ImagePullPolicy: corev1.PullIfNotPresent,
			Ports:           proxyCfg.ContainerPorts,
			Resources:       *proxyCfg.Resources,
		}
		if d.ObjectKey.Name == "edge" {
			proxyContainer.Name = "edge"
		}
		containers = append(containers, proxyContainer)
	}

	prev := obj.(*appsv1.Deployment)

	var update bool
	if prev.Labels["greymatter.io/greymatter-version"] != mesh.Spec.Version {
		update = true
	}

	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            d.ObjectKey.Name,
			Namespace:       mesh.Namespace,
			ResourceVersion: prev.ResourceVersion,
			Labels:          objectLabels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: matchLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: mesh.Spec.ImagePullSecret}},
					Containers:       containers,
				},
			},
		},
	}

	if svc == gmcore.JwtSecurity && svcCfg.VolumeMounts != nil {
		defaultMode := int32(420)
		deployment.Spec.Template.Spec.Volumes = []corev1.Volume{
			{
				Name: "jwt-users",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{Name: "jwt-users"},
						DefaultMode:          &defaultMode,
					},
				},
			},
		}
	}

	if svc == gmcore.Control {
		deployment.Spec.Template.Spec.ServiceAccountName = "control-pods"
	}

	return deployment, update
}
