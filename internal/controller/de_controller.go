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
	"github.com/clouddea/devs-go/simulation"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"net/rpc"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	interventionv1 "intervention-go/api/v1"
	corev1 "k8s.io/api/core/v1"
)

// DEReconciler reconciles a DE object
type DEReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=intervention.xue1.top,resources=des,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=intervention.xue1.top,resources=des/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=intervention.xue1.top,resources=des/finalizers,verbs=update

// 添加权限
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=pods/status,verbs=get
//+kubebuilder:rbac:groups="",resources=service,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DE object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *DEReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	instance := &interventionv1.DE{}
	if err := r.Client.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil // 说明已经被删除了，不需要再调度
		}
		return ctrl.Result{}, err
	}
	// 新增或更新（不考虑更新）
	// 获取pods
	existingPods := &corev1.PodList{}
	err := r.Client.List(ctx, existingPods, &client.ListOptions{
		Namespace: req.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			"app": instance.Name,
		}),
	})
	if err != nil {
		logger.Error(err, "获取已存在的Pod失败")
		return ctrl.Result{}, err
	}
	// 获取services
	existingServices := &corev1.ServiceList{}
	err = r.Client.List(ctx, existingServices, &client.ListOptions{
		Namespace: req.Namespace,
		LabelSelector: labels.SelectorFromSet(labels.Set{
			"app": instance.Name,
		}),
	})
	if err != nil {
		logger.Error(err, "获取已存在的Service失败")
		return ctrl.Result{}, err
	}
	// 获取pod map
	var existPodMap map[string]corev1.Pod = make(map[string]corev1.Pod)
	for _, pod := range existingPods.Items {
		if pod.GetObjectMeta().GetDeletionTimestamp() != nil {
			continue
		}
		existPodMap[pod.GetObjectMeta().GetName()] = pod
	}
	var existSvcMap map[string]corev1.Service = make(map[string]corev1.Service)
	for _, svc := range existingServices.Items {
		existSvcMap[svc.Name] = svc
	}
	// ensure resources
	readyCount := 0
	for _, comp := range instance.Spec.Components {
		ok1, ok2, ok3 := r.ensureComponent(instance, comp, existPodMap, existSvcMap)
		if ok1 {
			readyCount += 1
		}
		if !ok2 {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instance.Name + "-" + comp.Name + "-pod",
					Namespace: instance.Namespace,
					Labels: map[string]string{
						"app":  instance.Name,
						"comp": comp.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "busybox",
							Image: "busybox:glibc",
							Command: []string{
								"/bin/sh",
								"-c",
								"wget -O /root/app.bin " + comp.URL + "\n" +
									"chmod +x /root/app.bin\n" +
									"./root/app.bin",
							},
							StartupProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.IntOrString{
											Type:   intstr.Int,
											IntVal: 80,
										},
									},
								},
								FailureThreshold: 5,
								PeriodSeconds:    3,
							},
						},
					},
				},
				// 就绪探针与存活探针是相互独立的，当就绪探针失败时，不会接收Service的流量
				// 在论文中画流程图
				// 代码放在附录中
			}
			if err := controllerutil.SetControllerReference(instance, pod, r.Scheme); err != nil {
				logger.Error(err, "创建Pod引用失败")
				return ctrl.Result{}, err
			}

			if err := r.Client.Create(ctx, pod); err != nil {
				logger.Error(err, "创建Pod失败")
				return ctrl.Result{}, err
			}
		}
		if !ok3 {
			service := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      comp.Name,
					Namespace: instance.Namespace,
					Labels: map[string]string{
						"app": instance.Name,
					},
				},
				Spec: corev1.ServiceSpec{
					Selector: labels.Set{
						"app":  instance.Name,
						"comp": comp.Name,
					},
					Ports: []corev1.ServicePort{
						{TargetPort: intstr.IntOrString{Type: intstr.Int, IntVal: 80}, Port: 80},
					},
				},
			}
			if err := controllerutil.SetControllerReference(instance, service, r.Scheme); err != nil {
				logger.Error(err, "创建service引用失败")
				return ctrl.Result{}, err
			}

			if err := r.Client.Create(ctx, service); err != nil {
				logger.Error(err, "创建service失败")
				return ctrl.Result{}, err
			}
		}
	}
	if readyCount == len(instance.Spec.Components) {
		instance.Status.Status = "READY"
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			logger.Error(err, "更新状态失败")
			return ctrl.Result{}, err
		}
		if instance.Spec.AutoStart != "" {
			// 向最顶层发送请求并启动仿真
			// 构造rpc客户端，并发送请求
			client, err := rpc.DialHTTP("tcp", instance.Spec.AutoStart)
			if err != nil {
				logger.Error(err, "构建rpc客户端失败")
				instance2 := &interventionv1.DE{}
				if err := r.Client.Get(ctx, req.NamespacedName, instance2); err != nil {
					if errors.IsNotFound(err) {
						return ctrl.Result{}, nil // 说明已经被删除了，不需要再调度
					}
					return ctrl.Result{}, err
				}
				instance2.Status.Status = "FAILED"
				if err := r.Client.Status().Update(ctx, instance2); err != nil {
					logger.Error(err, "更新状态为failed失败") // 不允许对同一个版本作两次修改， 因此需要重新获取一下
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, err
			}
			var input = simulation.RootTimeArg{}
			var reply = simulation.RootTimeArg{}
			err = client.Call("Root.RPCSimulate", &input, &reply)
			if err != nil {
				logger.Error(err, "通过RPC启动仿真失败")
				instance2 := &interventionv1.DE{}
				if err := r.Client.Get(ctx, req.NamespacedName, instance2); err != nil {
					if errors.IsNotFound(err) {
						return ctrl.Result{}, nil // 说明已经被删除了，不需要再调度
					}
					return ctrl.Result{}, err
				}
				instance2.Status.Status = "FAILED"
				if err := r.Client.Status().Update(ctx, instance2); err != nil {
					logger.Error(err, "更新状态为failed2失败")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, err
			}
		}
	} else {
		instance.Status.Status = "PENDING"
		if err := r.Client.Status().Update(ctx, instance); err != nil {
			logger.Error(err, "更新状态为pending失败")
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *DEReconciler) ensureComponent(
	inst *interventionv1.DE,
	comp interventionv1.DEComponent,
	existPodMap map[string]corev1.Pod,
	existSvcMap map[string]corev1.Service) (bool, bool, bool) {
	serviceName := comp.Name
	podName := inst.Name + "-" + comp.Name + "-pod" // 不能有下划线
	var ok1 bool = false
	var ok2 bool = false
	_, ok1 = existPodMap[podName]
	_, ok2 = existSvcMap[serviceName]
	isPodRunning := existPodMap[podName].Status.Phase == corev1.PodRunning
	return ok1 && ok2 && isPodRunning, ok1, ok2
}

// SetupWithManager sets up the controller with the Manager.
func (r *DEReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&interventionv1.DE{}).
		Complete(r)
}
