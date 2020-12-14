package src
/**
* author: codeandcodex01
* email: codeandcodex@gmail.com
* data: 2020/05/01

* describe:
* two ways to deploy apps: one apply simple app/service root fold
* other one deploy form config app run list, run like this:
*
* ~ run apps: # cli [run]
*
* ~ run exact app: # cli nginx [namespace] [flag]
*
*/

import (
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/apimachinery/pkg/labels"
	"path/filepath"
	"github.com/ghodss/yaml"
	// "github.com/drone/envsubst"
	"net/url"
	"io/ioutil"
	"context"
	"sync"
	"os"
	"fmt"
	"log"
	"encoding/json"
	"strings"
	"reflect"
	"path"
	"time"
	"bytes"
)

type Config struct {
	Namespace string `json:"namespace"`
	Servicepath string `json:"servicepath"`
	Dbsrcname  string `json:"dbsrcname"`
	Nocheck []string    `json:"nocheck"`
	Apprun []string  `json:"apprun"`
	Dbuser string `json:"dbuser"`
	Dbpasswd string `json:"dbpasswd"`
	Dbname string `json:"dbname"`
	Debugfolder string `json:"debugfolder"`
}

var clientset *kubernetes.Clientset
var noCheckNodes, appRunNodes, dataInitFileMap map[string]string
var namespace, dbSrcName, servicePath, dbUser, dbPasswd, dbName, debugFolder string
var appConfig Config
var apps []string

const(
	CONFIGPATH = "./run"
)

//init
func init() {
	var config *rest.Config
	kubecfgpath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	//kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubecfgpath)
	if err != nil {
		kubecfgpath = filepath.Join("./run", "kubeconfig")
		config, err = clientcmd.BuildConfigFromFlags("", kubecfgpath)
		if err != nil {
			panic(err)
		}
	}
	//client set
	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		panic(err)
	}
	//init config
	initConfig()
	log.SetFlags(0)
}

//init config
func initConfig() {
	noCheckNodes,appRunNodes = make(map[string]string, 0), make(map[string]string, 0)
	deploycfgpath := filepath.Join(CONFIGPATH, "deployconfig.json")
	cfg, err := ioutil.ReadFile(deploycfgpath)
	err = json.Unmarshal(cfg, &appConfig)
	if err != nil {
		log.Println("warnning: config file not exist in ./run ! ")
		deploycfgpath = filepath.Join("./conf/dev", "deployconfig.json")
		cfg, err = ioutil.ReadFile(deploycfgpath)
		err = json.Unmarshal(cfg, &appConfig)
		if err != nil {
			log.Println("error: config file not exist in ./conf ! ")
		}
	}
	//set env ns
	os.Setenv("NAMESPACE", appConfig.Namespace)
	namespace = appConfig.Namespace
	//set db src name
	// dbSrcName = appConfig.Dbsrcname
	//set resource root
	servicePath = appConfig.Servicepath
	//set no check apps
	for _,v := range appConfig.Nocheck {
		noCheckNodes[v] = v
	}
	//set run apps 
	for _,v := range appConfig.Apprun {
		appRunNodes[v] = v
	}
	//app run list
	apps = appConfig.Apprun
	//set debug folder
	debugFolder = appConfig.Debugfolder
}

//get all resource files
func DeployResourceByLayNodes(app, strictModel, dbType, dbUrl, filePath, srcName, ns string) error {
	var layNodes []map[string]*ResNode
	if app == "all" {
		layNodes = GenerateDependTreeByConfig(servicePath, apps)
	}else{
		layNodes = GenerateDependTree(servicePath, app)	
	}

	//read data init file
	getDataInitFile()
	//set strict model
	if strictModel == "false" || strictModel == "debug" { //false
		layNodes = layNodes[:1]
	}else if strictModel == "reset" { //reset
		if app != "all" {
			layNodes = layNodes[:1]
		}
		//reset pods
		for _,apps := range layNodes {
			podReset(apps)
		}
		WriteDataStatusFile(dataInitFileMap, true)
		return nil
	}

	//deploy services
    for i:=len(layNodes)-1; i>=0 ; i-- {
    	wg := sync.WaitGroup{}
    	wg.Add(len(layNodes[i]))
    	for _,v := range layNodes[i] {
    		go func(path string, resNode *ResNode, dbUrl, filePath string) {
    			DeployAllResourceFiles(path, resNode, dbType, dbUrl, filePath, srcName)
    			wg.Done()
    		}(v.Res.Path, v, dbUrl, filePath)
    	}
    	wg.Wait()
    }
    //save data status
    WriteDataStatusFile(dataInitFileMap, true)
    return nil
}

//get all resource files
func DeployAllResourceFiles(pathname string, resNode *ResNode, dbType, dbUrl, filePath, srcName string) error {
    rd, err := ioutil.ReadDir(pathname)
    for _, fi := range rd {
        if fi.IsDir() {
            DeployAllResourceFiles(pathname +"/"+fi.Name(), resNode, dbType, dbUrl, filePath, srcName)
        } else {
        	filePath := pathname +"/"+fi.Name()
        	if path.Ext(filePath) == ".sql" {
        		DeployResource(filePath, resNode, dbType, dbUrl, srcName)
        	}
        }
    }
    return err
}

//deploy resource
func DeployResource(filePath string, resNode *ResNode, dbType, dbUrl, srcName string) {
	//parse url
	u, err := url.Parse(dbUrl)
    if err != nil {
        panic(err)
    }

    dbUser = u.User.Username()
    dbPasswd, _ = u.User.Password()
    //get db name
    dbNameSlice := strings.Split(u.Path, "/")
    dbName = dbNameSlice[1]
	//get resource type
	switch(dbType) {
	case "POSTGRES":
		break
	case "MYSQL":
		dbSrcName = srcName
		//init data
		if len(resNode.DataInitPath)>0 {
			dbDeployments:= getDeployments(dbSrcName)
			if len(dbDeployments) >0 {
				if checkPodStatus(*dbDeployments[0])  {
					pods := getPodsByLabel(dbSrcName)
					podName := pods.Items[0].Name
					for i:=len(resNode.DataInitPath)-1 ;i>=0;i-- {
						queryBytes, _ := ioutil.ReadFile(resNode.DataInitPath[i])
						query := string(queryBytes)
						query = strings.Replace(query, "`", "\\`", -1)
						if _, sqlExist := dataInitFileMap[resNode.DataInitPath[i]]; sqlExist {
							if dataInitFileMap[resNode.DataInitPath[i]] == "false" {
								// PrintLog()
								log.Print(resNode.DataInitPath[i])
								dataInitFileMap[resNode.DataInitPath[i]] = "true"
								initPodData("sql", namespace, resNode.DataInitPath[i], podName, dbSrcName, dbUser, dbPasswd, dbName)
							}
						}else{
							initPodData("sql", namespace, resNode.DataInitPath[i], podName, dbSrcName, dbUser, dbPasswd, dbName)
							dataInitFileMap[resNode.DataInitPath[i]] = "true"
						}
						
					}
				}
			}else {
				log.Fatalln(dbSrcName, "service not found !")
			}
		}

		break
	case "REDIS":
		break
	case "SQLITE3":
		break
	default:
		break
	}
}

//get resource yaml
func getResourceYaml(filePath string) []byte {
	bytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		panic(err.Error())
	}
	return bytes
}

//getResourceType
func getResourceType(bytes []byte) string{
	typeJson, err := yaml.YAMLToJSON(bytes)
	if err != nil {
		panic("get resource type error !")
	}
	var typeIn interface{}
	json.Unmarshal(typeJson, &typeIn)
	return typeIn.(map[string]interface{})["kind"].(string)
}

//init pod data
func initPodData(dataType string, namespace, path, padName, containerName, dbUser, dbPasswd, dbName string) {
	queryBytes, _ := ioutil.ReadFile(path)
	query := string(queryBytes)
	query = strings.Replace(query, "`", "\\`", -1)
	querySlice := strings.Split(query, ";")
	sql := ""
	for k, queryStr := range querySlice {
		if len(queryStr) <2 {continue}
		sql = sql +";"+ queryStr
		if k%50 == 0 && k >0 {
			sql = sql + ";"
			command := "mysql -u"+dbUser+" -p"+dbPasswd+" "+dbName+" -e \""+string(sql)+"\""
			_, _, err := podExecCommand(namespace, padName, command, containerName)
			if err != nil {
				// log.Println("data exists or sql error!")
			}
			sql = ""
		}
	}
	sql = sql + ";"
	command := "mysql -u"+dbUser+" -p"+dbPasswd+" "+dbName+" -e \""+string(sql)+"\""
	_, _, err := podExecCommand(namespace, padName, command, containerName)
	if err != nil {
		// log.Println("data exists or sql error!")
	}
}

//get data init file map
func getDataInitFile() {
	dataInitFileMap = make(map[string]string, 0)
	dataBytes := ReadDataFile("./run/data-init-status.json")
	json.Unmarshal(dataBytes, &dataInitFileMap)
}

//check pod status
func checkPodStatus(deploy appsv1.Deployment) bool{
	checkStatus := false
	for {
		deploys := getDeployments(deploy.Name)
		if len(deploys) >0 {
			if deploys[0].Status.Replicas >0 {
				for _, condition := range deploys[0].Status.Conditions {
					if condition.Type == "Available" && condition.Status == "True" {
						checkStatus = true
						break
					}
				}
			}
		}

		if checkStatus {
			return true
		}
		fmt.Print(".")
		time.Sleep(2*time.Second)
	}

	return false
}

//deploy resource ------------------------------------

//decode Service
func Service(bytes []byte) corev1.Service{
	var spec corev1.Service
	err := yaml.Unmarshal(bytes, &spec)
	if err != nil {
		panic(err.Error())
	}
	return spec
}

//deploy service
func deployService(svc corev1.Service) error{
	_, err := clientset.CoreV1().Services(namespace).Create(context.TODO(), &svc, metav1.CreateOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			existSvc := getServices(svc.Name)
			resourceVersion := existSvc[0].ObjectMeta.ResourceVersion
			clusterIP := existSvc[0].Spec.ClusterIP
			svc.ObjectMeta.ResourceVersion = resourceVersion
			svc.Spec.ClusterIP = clusterIP
			_, errUpdate := clientset.CoreV1().Services(namespace).Update(context.TODO(), &svc, metav1.UpdateOptions{})
			if errUpdate !=nil {
				log.Println("failed services updated error!", errUpdate)
			}else{
				PrintLog()
				log.Print("success services        ", "\""+svc.Name+"\"")
			}
		}
		return err
	}else{
		PrintLog()
		log.Print("success services        ", "\""+svc.Name+"\"")
		return nil
	}
}

//get services
func getServices(apps ...string) []*corev1.Service {
	var svcs []*corev1.Service
	if len(apps) > 0 {
		for _, app := range apps {
			svc, _ := clientset.CoreV1().Services(namespace).Get(context.TODO(), app, metav1.GetOptions{})
			if svc.Name == "" {
				log.Println("service not exists!")
			}else{
				svcs = append(svcs, svc)
			}
		}
	}else{
		svcList, _ := clientset.CoreV1().Services(namespace).List(context.TODO(), metav1.ListOptions{})
		fmt.Printf("there are %d svc in the cluster\n", len(svcList.Items))
		for _, svc := range svcList.Items {
			svcs = append(svcs, &svc)
		}
	}
	return svcs
}

//decode Deployment
func Deployment(bytes []byte) appsv1.Deployment{
	var spec appsv1.Deployment
	err := yaml.Unmarshal(bytes, &spec)
	if err != nil {
		panic(err.Error())
	}
	return spec
}

//deploy deployment
func deployDeployment(deploy appsv1.Deployment) error{
	_, err := clientset.AppsV1().Deployments(namespace).Create(context.TODO(), &deploy, metav1.CreateOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			existDeploys := getDeployments(deploy.Name)
			if !reflect.DeepEqual(existDeploys[0], deploy) {
				deploymentUpdate, errUpdate := clientset.AppsV1().Deployments(namespace).Update(context.TODO(), &deploy, metav1.UpdateOptions{})
				if errUpdate !=nil {
					log.Println("failed deployments updated error!")
				}else{
					if deploymentUpdate.Status.Replicas > 0 {
						PrintLog()
						log.Print("success deployments     ", "\""+deploy.Name+"\"")
						// log.Println("~~~:replicas:", deploymentUpdate.Status.Replicas)
						// for _, st := range deploymentUpdate.Status.Conditions {
						// 	log.Println("~~~:conditions:", "->", st.Type, ":", st.Status)
						// }
					}
				}
			}else{
				PrintLog()
				log.Print("no need update")
			}
		}
		return err
	}else{
		PrintLog()
		log.Print("success deployments     ", "\""+deploy.Name+"\"")
		return nil
	}
}

//get deploys
func getDeployments(apps ...string) []*appsv1.Deployment{
	var deploys []*appsv1.Deployment
	if len(apps) > 0 {
		for _, app := range apps {
			deploy, _ := clientset.AppsV1().Deployments(namespace).Get(context.TODO(), app, metav1.GetOptions{})
			if deploy.Status.Replicas == 0 {
				// log.Println("resource not found!")
			}else{
				// log.Println("resource already exists!")
				deploys = append(deploys, deploy)
			}
		}
	}else{
		deployList, _ := clientset.AppsV1().Deployments(namespace).List(context.TODO(), metav1.ListOptions{})
		fmt.Printf("there are %d deployment in the cluster\n", len(deployList.Items))
		for _, deploy := range deployList.Items {
			deploys = append(deploys, &deploy)
		}
	}
	return deploys	
}

//decode ConfigMap
func ConfigMap(bytes []byte) corev1.ConfigMap{
	var spec corev1.ConfigMap
	err := yaml.Unmarshal(bytes, &spec)
	if err != nil {
		panic(err.Error())
	}
	return spec
}

//deploy Configmap
func deployConfigMap(cm corev1.ConfigMap) error{
	_, err := clientset.CoreV1().ConfigMaps(namespace).Create(context.TODO(), &cm, metav1.CreateOptions{})
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			_, errUpdate := clientset.CoreV1().ConfigMaps(namespace).Update(context.TODO(), &cm, metav1.UpdateOptions{})
			if errUpdate !=nil {
				log.Println("failed configmaps updated error!")
			}else{
				PrintLog()
				log.Print("success configmaps      ", "\""+cm.Name+"\"")
			}
		}
		return err
	}else{
		PrintLog()
		log.Print("success configmaps      ", "\""+cm.Name+"\"")
		return nil
	}
}
	
//get Configmaps
func getConfigMaps() {
	configmaps, _ := clientset.CoreV1().ConfigMaps(namespace).List(context.TODO(), metav1.ListOptions{})
	fmt.Printf("there are %d cm in the cluster\n", len(configmaps.Items))
	for _, cm := range configmaps.Items {
		log.Println(cm.Name)
	}
}

//get pods by label
func getPodsByLabel(app string) *corev1.PodList{
	labelSelector := metav1.LabelSelector{MatchLabels: map[string]string{"app": "mariadb"}}
    listOptions := metav1.ListOptions{
        LabelSelector: labels.Set(labelSelector.MatchLabels).String(),
        Limit:         100,
    }
	pods, _:= clientset.CoreV1().Pods(namespace).List(context.TODO(), listOptions)
	return pods
}

//deploy tools resource ------------------------------------
//pod exec command
func podExecCommand(namespace, podName, command, containerName string) (string, string, error) {
	kubecfgpath := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	//kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubecfgpath)
	if err != nil {
		kubecfgpath = filepath.Join("./run/", "kubeconfig")
		config, err = clientcmd.BuildConfigFromFlags("", kubecfgpath)
		if err != nil {
			panic(err)
		}
	}

	k8sCli, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", "", err
	}

	//command
	cmd := []string{
		"sh",
		"-c",
		command,
	}
	const tty = false
	req := k8sCli.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).SubResource("exec").Param("container", containerName)
	req.VersionedParams(
		&corev1.PodExecOptions{
			Command: cmd,
			Stdin:   false,
			Stdout:  true,
			Stderr:  true,
			TTY:     tty,
		},
		scheme.ParameterCodec,
	)

	var stdout, stderr bytes.Buffer
	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		return "", "", err
	}
	err = exec.Stream(remotecommand.StreamOptions{
		Stdin:  nil,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return "", "", err
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

//pod reset
func podReset(resNodeMap map[string]*ResNode) {
	//scan resource file
	for _, resNode := range resNodeMap {
		scanResFile(resNode.Res.Path)
		//update data init file
		for i:=len(resNode.DataInitPath)-1 ;i>=0;i-- {
			dataInitFileMap[resNode.DataInitPath[i]] = "false"
		}
	}
}

//scan data file
func scanResFile(pathName string) {
	rd, err := ioutil.ReadDir(pathName)
	if err != nil {
    	log.Fatalln("scan data file - read file error!", err)
    }
    //resource delete
	for _, fi := range rd {
		if !fi.IsDir() {
			dataExt := path.Ext(fi.Name())
			switch(dataExt) {
			case ".yaml":
				bytes := getResourceYaml(pathName+"/"+fi.Name())
				resourceType := getResourceType(bytes)
				resDelete(resourceType, bytes)
				break
			}
		}else{
			scanResFile(pathName+"/"+fi.Name())
		}
	 }
}


//resource delete
func resDelete(resourceType string, bytes []byte) {
	switch(resourceType) {
	case "Service":
		name := Service(bytes).Name
		err := clientset.CoreV1().Services(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			log.Println("delete service error!", err)
		}
		log.Print("delete success services      ", "\""+name+"\"")
		break
	case "Deployment":
		name := Deployment(bytes).Name
		err := clientset.AppsV1().Deployments(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			log.Println("delete deployment error!", err)
		}
		log.Print("delete success deployment    ", "\""+name+"\"")
		break
	case "ConfigMap":
		name := ConfigMap(bytes).Name
		err := clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), name, metav1.DeleteOptions{})
		if err != nil {
			log.Println("delete configmap error!", err)
		}
		log.Print("delete success configmap     ", "\""+name+"\"")
		break
	case "StatefulSet":
		break
	}
}










