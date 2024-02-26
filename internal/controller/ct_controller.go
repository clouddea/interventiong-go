/*
Copyright 2024.

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

package controller

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"golang.org/x/crypto/ssh"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	interventionv1 "intervention-go/api/v1"
)

// CTReconciler reconciles a CT object
type CTReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=intervention.xue1.top,resources=cts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=intervention.xue1.top,resources=cts/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=intervention.xue1.top,resources=cts/finalizers,verbs=update

//+kubebuilder:rbac:groups="",resources=configmap,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=deployment,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="apps",resources=statefulset,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CT object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *CTReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	instance := &interventionv1.CT{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil // 说明已经被删除了，不需要再调度
		}
		logger.Error(err, "获取实例信息失败")
		return ctrl.Result{}, err
	}
	configMap, err := r.ensureConfigMap(instance)
	if err != nil {
		logger.Error(err, "操作configmap失败")
		return ctrl.Result{}, err
	}
	deployment, err := r.ensureDeployment(configMap, instance)
	if err != nil {
		logger.Error(err, "操作deployment失败")
		return ctrl.Result{}, err
	}
	instance.Status.Daemon = deployment.ObjectMeta.Name

	statefulSet, err := r.ensureStatefulSet(configMap, instance)
	if err != nil {
		logger.Error(err, "操作statefulset失败")
		return ctrl.Result{}, err
	}
	instance.Status.Mpich = statefulSet.ObjectMeta.Name

	if *(statefulSet.Spec.Replicas) != instance.Spec.Replicas {
		err := r.Client.Delete(ctx, deployment)
		if err != nil {
			return ctrl.Result{}, err
		}
		statefulSet.Spec.Replicas = &instance.Spec.Replicas
		err = r.Client.Update(ctx, statefulSet)
		if err != nil {
			return ctrl.Result{}, err
		}
		instance.Status.Daemon = ""
	}
	if err := r.Client.Status().Update(ctx, instance); err != nil {
		logger.Error(err, "更新状态失败")
		return ctrl.Result{}, err
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *CTReconciler) ensureConfigMap(instance *interventionv1.CT) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{}
	key := types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name + "-config"}
	if err := r.Client.Get(context.TODO(), key, configMap); err != nil {
		if errors.IsNotFound(err) {
			configMap.ObjectMeta = metav1.ObjectMeta{
				Namespace: key.Namespace,
				Name:      key.Name,
			}
			public, private, err := MakeSSHKeyPair()
			if err != nil {
				return nil, err
			}
			configMap.Data = map[string]string{
				"id_rsa":          private,
				"id_rsa.pub":      public,
				"authorized_keys": public,
				"config":          "Host *\n    StrictHostKeyChecking no",
			}
			if err := controllerutil.SetControllerReference(instance, configMap, r.Scheme); err != nil {
				fmt.Println("创建configmap引用失败")
				return nil, err
			}
			if err := r.Create(context.TODO(), configMap); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	return configMap, nil
}

func (r *CTReconciler) ensureDeployment(config *corev1.ConfigMap, instance *interventionv1.CT) (*v1.Deployment, error) {
	service := &corev1.Service{}
	key := types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name + "-ct-daemon"}
	if err := r.Client.Get(context.TODO(), key, service); err != nil {
		if errors.IsNotFound(err) {
			service.ObjectMeta = metav1.ObjectMeta{
				Namespace: key.Namespace,
				Name:      key.Name,
			}
			service.Spec = corev1.ServiceSpec{
				ClusterIP: "None",
				Selector: map[string]string{
					"app": key.Name,
				},
			}
			if err := controllerutil.SetControllerReference(instance, service, r.Scheme); err != nil {
				fmt.Println("创建deployment service引用失败")
				return nil, err
			}
			if err := r.Create(context.TODO(), service); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	deployment := &v1.Deployment{}
	if err := r.Client.Get(context.TODO(), key, deployment); err != nil {
		if errors.IsNotFound(err) {
			deployment.ObjectMeta = metav1.ObjectMeta{
				Namespace: key.Namespace,
				Name:      key.Name,
			}
			var replicas int32 = 1
			var privileged bool = true
			deployment.Spec = v1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": key.Name,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": key.Name,
						},
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "ssh",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: config.ObjectMeta.Name,
										},
									},
								},
							},
						},
						Containers: []corev1.Container{
							{
								Name:            key.Name,
								Image:           "ct-daemon:1.0",
								ImagePullPolicy: corev1.PullNever,
								Command: []string{
									"/bin/sh",
									"-c",
									fmt.Sprintf("hostname %v", key.Name) + "\n" +
										"mkdir /root/.ssh\n" +
										"cp /root/ssh/* /root/.ssh\n" +
										"chmod -R 600 /root/.ssh\n" +
										"service ssh start\n" +
										"/project/daemon.exe\n",
								},
								Env: []corev1.EnvVar{
									{
										Name:  "CLUSTER_SIZE_ENV",
										Value: strconv.Itoa(int(instance.Spec.Replicas)),
									},
									{
										Name:  "CLUSTER_NAME_ENV",
										Value: instance.Name + "-ct-mpich",
									},
									{
										Name:  "CLUSTER_SERVICE_NAME_ENV",
										Value: instance.Name + "-ct-mpich",
									},
								},
								SecurityContext: &corev1.SecurityContext{
									Privileged: &privileged,
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "ssh",
										MountPath: "/root/ssh",
										ReadOnly:  true,
									},
								},
							},
						},
					},
				},
			}

			if err := controllerutil.SetControllerReference(instance, deployment, r.Scheme); err != nil {
				fmt.Println("创建deployment引用失败")
				return nil, err
			}
			if err := r.Create(context.TODO(), deployment); err != nil {
				return nil, err
			}
		}
	}

	return deployment, nil
}

func (r *CTReconciler) ensureStatefulSet(config *corev1.ConfigMap, instance *interventionv1.CT) (*v1.StatefulSet, error) {
	service := &corev1.Service{}
	key := types.NamespacedName{Namespace: instance.Namespace, Name: instance.Name + "-ct-mpich"}
	if err := r.Client.Get(context.TODO(), key, service); err != nil {
		if errors.IsNotFound(err) {
			service.ObjectMeta = metav1.ObjectMeta{
				Namespace: key.Namespace,
				Name:      key.Name,
			}
			service.Spec = corev1.ServiceSpec{
				ClusterIP: "None",
				Selector: map[string]string{
					"app": key.Name,
				},
			}
			if err := controllerutil.SetControllerReference(instance, service, r.Scheme); err != nil {
				fmt.Println("创建statefulset service引用失败")
				return nil, err
			}
			if err := r.Create(context.TODO(), service); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	statefulset := &v1.StatefulSet{}
	if err := r.Client.Get(context.TODO(), key, statefulset); err != nil {
		if errors.IsNotFound(err) {
			statefulset.ObjectMeta = metav1.ObjectMeta{
				Namespace: key.Namespace,
				Name:      key.Name,
			}
			var replicas int32 = instance.Spec.Replicas
			var privileged bool = true
			statefulset.Spec = v1.StatefulSetSpec{
				ServiceName: key.Name,
				Replicas:    &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": key.Name,
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": key.Name,
						},
					},
					Spec: corev1.PodSpec{
						Volumes: []corev1.Volume{
							{
								Name: "ssh",
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: config.ObjectMeta.Name,
										},
									},
								},
							},
						},
						Containers: []corev1.Container{
							{
								Name:            key.Name,
								Image:           "ct-mpich:1.0",
								ImagePullPolicy: corev1.PullNever,
								Command: []string{
									"/bin/sh",
									"-c",
									fmt.Sprintf("hostname %v", key.Name) + "\n" +
										"mkdir /root/.ssh\n" +
										"cp /root/ssh/* /root/.ssh\n" +
										"chmod -R 600 /root/.ssh\n" +
										"service ssh start\n" +
										"sleep 24d\n",
								},
								SecurityContext: &corev1.SecurityContext{
									Privileged: &privileged,
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "ssh",
										MountPath: "/root/ssh",
										ReadOnly:  true,
									},
								},
							},
						},
					},
				},
			}
			if err := controllerutil.SetControllerReference(instance, statefulset, r.Scheme); err != nil {
				fmt.Println("创建statefulset引用失败")
				return nil, err
			}
			if err := r.Create(context.TODO(), statefulset); err != nil {
				return nil, err
			}
		}
	}

	return statefulset, nil
}

/**产生一对公钥和私钥*/
/** 用法：
public, private, _ := MakeSSHKeyPair()
fmt.Println(private)
fmt.Println(public)
*/
func MakeSSHKeyPair() (string, string, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		return "", "", err
	}

	// generate and write private key as PEM
	var privKeyBuf strings.Builder

	privateKeyPEM := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(privateKey)}
	if err := pem.Encode(&privKeyBuf, privateKeyPEM); err != nil {
		return "", "", err
	}

	// generate and write public key
	pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", err
	}

	var pubKeyBuf strings.Builder
	pubKeyBuf.Write(ssh.MarshalAuthorizedKey(pub))

	return pubKeyBuf.String(), privKeyBuf.String(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CTReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&interventionv1.CT{}).
		Complete(r)
}
