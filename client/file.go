package client

import (
	"cess-portal/conf"
	"cess-portal/internal/chain"
	"cess-portal/internal/logger"
	"cess-portal/internal/rpc"
	"cess-portal/module"
	"cess-portal/tools"
	"context"
	"errors"
	"fmt"
	"github.com/btcsuite/btcutil/base58"
	"google.golang.org/protobuf/proto"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

/*
FileUpload means upload files to CESS system
path:The absolute path of the file to be uploaded
backups:Number of backups of files that need to be uploaded
PrivateKey:Encrypted password for uploaded files
*/
func FileUpload(path, backups, PrivateKey string) {
	chain.Chain_Init()
	file, err := os.Stat(path)
	if err != nil {
		fmt.Printf("[Error]Please enter the correct file path!\n")
		return
	}

	if file.IsDir() {
		fmt.Printf("[Error]Please do not upload the folder!\n")
		return
	}

	spares, err := strconv.Atoi(backups)
	if err != nil {
		fmt.Printf("[Error]Please enter a correct integer!\n")
		return
	}

	filehash, err := tools.CalcFileHash(path)
	if err != nil {
		fmt.Printf("[Error]There is a problem with the file, please replace it!\n")
		return
	}

	fileid, err := tools.GetGuid(1)
	if err != nil {
		fmt.Printf("[Error]Create snowflake fail! error:%s\n", err)
		return
	}
	var blockinfo module.FileUploadInfo
	blockinfo.Backups = backups
	blockinfo.FileId = fileid
	blockinfo.BlockSize = int32(file.Size())
	blockinfo.FileHash = filehash

	blocksize := 1024 * 1024
	blocktotal := 0

	f, err := os.Open(path)
	if err != nil {
		fmt.Println("[Error]This file was broken! ", err)
		return
	}
	defer f.Close()
	filebyte, err := ioutil.ReadAll(f)
	if err != nil {
		fmt.Println("[Error]analyze this file error! ", err)
		return
	}

	var ci chain.CessInfo
	ci.RpcAddr = conf.ClientConf.ChainData.CessRpcAddr
	ci.ChainModule = chain.FindSchedulerInfoModule
	ci.ChainModuleMethod = chain.FindSchedulerInfoMethod
	schds, err := ci.GetSchedulerInfo()
	if err != nil {
		fmt.Println("[Error]Get scheduler randomly error! ", err)
		return
	}
	filesize := new(big.Int)
	fee := new(big.Int)

	ci.IdentifyAccountPhrase = conf.ClientConf.ChainData.IdAccountPhraseOrSeed
	ci.TransactionName = chain.UploadFileTransactionName

	if file.Size()/1024 == 0 {
		filesize.SetInt64(1)
	} else {
		filesize.SetInt64(file.Size() / 1024)
	}
	fee.SetInt64(int64(0))

	AsInBlock, err := ci.UploadFileMetaInformation(fileid, file.Name(), filehash, PrivateKey == "", uint8(spares), filesize, fee)
	if err != nil {
		fmt.Printf("\n[Error]Upload file meta information error:%s\n", err)
		return
	}
	fmt.Printf("\nFile meta info upload:%s ,fileid is:%s\n", AsInBlock, fileid)

	var client *rpc.Client
	for i, schd := range schds {
		wsURL := "ws://" + string(base58.Decode(string(schd.Ip)))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		client, err = rpc.DialWebsocket(ctx, wsURL, "")
		defer cancel()
		if err != nil {
			err = errors.New("Connect with scheduler timeout")
			fmt.Printf("%s[Tips]%sdialog with scheduler:%s fail! reason:%s\n", tools.Yellow, tools.Reset, string(base58.Decode(string(schd.Ip))), err)
			if i == len(schds)-1 {
				fmt.Printf("%s[Error]All scheduler is offline!!!%s\n", tools.Red, tools.Reset)
				logger.OutPutLogger.Sugar().Infof("\n%s[Error]All scheduler is offlien!!!%s\n", tools.Red, tools.Reset)
				return
			}
			continue
		} else {
			break
		}
	}
	sp := sync.Pool{
		New: func() interface{} {
			return &rpc.ReqMsg{}
		},
	}
	commit := func(num int, data []byte) {
		blockinfo.BlockNum = int32(num) + 1
		blockinfo.Data = data
		info, err := proto.Marshal(&blockinfo)
		if err != nil {
			fmt.Println("[Error]Serialization error, please upload again! ", err)
			logger.OutPutLogger.Sugar().Infof("[Error]Serialization error, please upload again! ", err)
			return
		}
		reqmsg := sp.Get().(*rpc.ReqMsg)
		reqmsg.Body = info
		reqmsg.Method = module.UploadService
		reqmsg.Service = module.CtlServiceName

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		resp, err := client.Call(ctx, reqmsg)
		defer cancel()
		if err != nil {
			fmt.Printf("\n%s[Error]Failed to transfer file to scheduler,error:%s%s\n", tools.Red, err, tools.Reset)
			logger.OutPutLogger.Sugar().Infof("%s[Error]Failed to transfer file to scheduler,error:%s%s\n", tools.Red, err, tools.Reset)
			os.Exit(conf.Exit_SystemErr)
		}

		var res rpc.RespBody
		err = proto.Unmarshal(resp.Body, &res)
		if err != nil {
			fmt.Printf("\n[Error]Error getting reply from schedule, transfer failed! ", err)
			logger.OutPutLogger.Sugar().Infof("[Error]Error getting reply from schedule, transfer failed! ", err)
			os.Exit(conf.Exit_SystemErr)
		}
		if res.Code != 0 {
			fmt.Printf("\n[Error]Upload file fail!scheduler problem:%s\n", res.Msg)
			logger.OutPutLogger.Sugar().Infof("\n[Error]Upload file fail!scheduler problem:%s\n", res.Msg)
			os.Exit(conf.Exit_SystemErr)
		}
		sp.Put(reqmsg)
	}

	if len(PrivateKey) != 0 {
		_, err = os.Stat(conf.ClientConf.PathInfo.KeyPath)
		if err != nil {
			err = os.Mkdir(conf.ClientConf.PathInfo.KeyPath, os.ModePerm)
			if err != nil {
				fmt.Printf("%s[Error]Create key path error :%s%s\n", tools.Red, err, tools.Reset)
				logger.OutPutLogger.Sugar().Infof("%s[Error]Create key path error :%s%s\n", tools.Red, err, tools.Reset)
				os.Exit(conf.Exit_SystemErr)
			}
		}

		os.Create(filepath.Join(conf.ClientConf.PathInfo.KeyPath, file.Name()) + ".pem")
		keyfile, err := os.OpenFile(filepath.Join(conf.ClientConf.PathInfo.KeyPath, file.Name())+".pem", os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			fmt.Printf("%s[Error]:Failed to save key%s error:%s\n", tools.Red, tools.Reset, err)
			logger.OutPutLogger.Sugar().Infof("%s[Error]:Failed to save key%s error:%s\n", tools.Red, tools.Reset, err)
			return
		}
		_, err = keyfile.WriteString(PrivateKey)
		if err != nil {
			fmt.Printf("%s[Error]:Failed to write key to file:%s%s error:%s", tools.Red, filepath.Join(conf.ClientConf.PathInfo.KeyPath, (file.Name()+".pem")), tools.Reset, err)
			logger.OutPutLogger.Sugar().Infof("%s[Error]:Failed to write key to file:%s%s error:%s", tools.Red, filepath.Join(conf.ClientConf.PathInfo.KeyPath, (file.Name()+".pem")), tools.Reset, err)
			return
		}

		encodefile, err := tools.AesEncrypt(filebyte, []byte(PrivateKey))
		if err != nil {
			fmt.Println("[Error]Encode the file fail ,error! ", err)
			return
		}
		blocks := len(encodefile) / blocksize
		if len(encodefile)%blocksize == 0 {
			blocktotal = blocks
		} else {
			blocktotal = blocks + 1
		}
		blockinfo.Blocks = int32(blocktotal)
		var bar tools.Bar
		bar.NewOption(0, int64(blocktotal))
		for i := 0; i < blocktotal; i++ {
			block := make([]byte, 0)
			if blocks != i {
				block = encodefile[i*blocksize : (i+1)*blocksize]
				bar.Play(int64(i + 1))
			} else {
				block = encodefile[i*blocksize:]
				bar.Play(int64(i + 1))
			}
			commit(i, block)
		}
		bar.Finish()
	} else {
		fmt.Printf("%s[Tips]%s:upload file:%s without private key", tools.Yellow, tools.Reset, path)
		blocks := len(filebyte) / blocksize
		if len(filebyte)%blocksize == 0 {
			blocktotal = blocks
		} else {
			blocktotal = blocks + 1
		}
		blockinfo.Blocks = int32(blocktotal)
		var bar tools.Bar
		bar.NewOption(0, int64(blocktotal))
		for i := 0; i < blocktotal; i++ {
			block := make([]byte, 0)
			if blocks != i {
				block = filebyte[i*blocksize : (i+1)*blocksize]
				bar.Play(int64(i + 1))
			} else {
				block = filebyte[i*blocksize:]
				bar.Play(int64(i + 1))
			}
			commit(i, block)
		}
		bar.Finish()
	}
}

/*
FileDownload means download file by file id
fileid:fileid of the file to download
*/
func FileDownload(fileid string) {
	chain.Chain_Init()
	var ci chain.CessInfo
	ci.RpcAddr = conf.ClientConf.ChainData.CessRpcAddr
	ci.ChainModule = chain.FindFileChainModule
	ci.ChainModuleMethod = chain.FindFileModuleMethod[0]
	fileinfo, err := ci.GetFileInfo(fileid)
	if err != nil {
		fmt.Printf("%s[Error]Get file:%s info fail:%s%s\n", tools.Red, fileid, err, tools.Reset)
		logger.OutPutLogger.Sugar().Infof("%s[Error]Get file:%s info fail:%s%s\n", tools.Red, fileid, err, tools.Reset)
		return
	}
	if string(fileinfo.FileState) != "active" {
		fmt.Printf("%s[Tips]The file:%s has not been backed up, please try again later%s\n", tools.Yellow, fileid, tools.Reset)
		logger.OutPutLogger.Sugar().Infof("%s[Tips]The file:%s has not been backed up, please try again later%s\n", tools.Yellow, fileid, tools.Reset)
		return
	}
	if fileinfo.File_Name == nil {
		fmt.Printf("%s[Error]The fileid:%s used to find the file is incorrect, please try again%s\n", tools.Red, fileid, err, tools.Reset)
		logger.OutPutLogger.Sugar().Infof("%s[Error]The fileid:%s used to find the file is incorrect, please try again%s\n", tools.Red, fileid, err, tools.Reset)
		return
	}

	_, err = os.Stat(conf.ClientConf.PathInfo.InstallPath)
	if err != nil {
		err = os.Mkdir(conf.ClientConf.PathInfo.InstallPath, os.ModePerm)
		if err != nil {
			fmt.Printf("%s[Error]Create install path error :%s%s\n", tools.Red, err, tools.Reset)
			logger.OutPutLogger.Sugar().Infof("%s[Error]Create install path error :%s%s\n", tools.Red, err, tools.Reset)
			os.Exit(conf.Exit_SystemErr)
		}
	}
	_, err = os.Create(filepath.Join(conf.ClientConf.PathInfo.InstallPath, string(fileinfo.File_Name[:])))
	if err != nil {
		fmt.Printf("%s[Error]Create installed file error :%s%s\n", tools.Red, err, tools.Reset)
		logger.OutPutLogger.Sugar().Infof("%s[Error]Create installed file error :%s%s\n", tools.Red, err, tools.Reset)
		os.Exit(conf.Exit_SystemErr)
	}
	installfile, err := os.OpenFile(filepath.Join(conf.ClientConf.PathInfo.InstallPath, string(fileinfo.File_Name[:])), os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("%s[Error]:Failed to save key error:%s%s", tools.Red, err, tools.Reset)
		logger.OutPutLogger.Sugar().Infof("%s[Error]:Failed to save key error:%s%s", tools.Red, err, tools.Reset)
		return
	}
	defer installfile.Close()

	ci.RpcAddr = conf.ClientConf.ChainData.CessRpcAddr
	ci.ChainModule = chain.FindSchedulerInfoModule
	ci.ChainModuleMethod = chain.FindSchedulerInfoMethod
	schds, err := ci.GetSchedulerInfo()
	if err != nil {
		fmt.Printf("%s[Error]Get scheduler list error:%s%s\n ", tools.Red, err, tools.Reset)
		return
	}

	var client *rpc.Client
	for i, schd := range schds {
		wsURL := "ws://" + string(base58.Decode(string(schd.Ip)))
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		client, err = rpc.DialWebsocket(ctx, wsURL, "")
		defer cancel()
		if err != nil {
			err = errors.New("Connect with scheduler timeout")
			fmt.Printf("%s[Tips]%sdialog with scheduler:%s fail! reason:%s\n", tools.Yellow, tools.Reset, string(base58.Decode(string(schd.Ip))), err)
			if i == len(schds)-1 {
				fmt.Printf("%s[Error]All scheduler is offline!!!%s\n", tools.Red, tools.Reset)
				//logger.OutPutLogger.Sugar().Infof("\n%s[Error]All scheduler is offlien!!!%s\n", tools.Red, tools.Reset)
				return
			}
			continue
		} else {
			break
		}
	}

	var wantfile module.FileDownloadReq
	var bar tools.Bar
	var getAllBar sync.Once
	sp := sync.Pool{
		New: func() interface{} {
			return &rpc.ReqMsg{}
		},
	}
	wantfile.FileId = fileid
	wantfile.WalletAddress = conf.ClientConf.ChainData.AccountPublicKey
	wantfile.Blocks = 1

	for {
		data, err := proto.Marshal(&wantfile)
		if err != nil {
			fmt.Printf("[Error]Marshal req file error:%s\n", err)
			logger.OutPutLogger.Sugar().Infof("[Error]Marshal req file error:%s\n", err)
			return
		}
		req := sp.Get().(*rpc.ReqMsg)
		req.Method = module.DownloadService
		req.Service = module.CtlServiceName
		req.Body = data

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		resp, err := client.Call(ctx, req)
		cancel()
		if err != nil {
			fmt.Printf("[Error]Download file fail error:%s\n", err)
			logger.OutPutLogger.Sugar().Infof("[Error]Download file fail error:%s\n", err)
			return
		}

		var respbody rpc.RespBody
		err = proto.Unmarshal(resp.Body, &respbody)
		if err != nil || respbody.Code != 0 {
			fmt.Printf("[Error]Download file from CESS error:%s. reply message:%s\n", err, respbody.Msg)
			logger.OutPutLogger.Sugar().Infof("[Error]Download file from CESS error:%s. reply message:%s\n", err, respbody.Msg)
			return
		}
		var blockData module.FileDownloadInfo
		err = proto.Unmarshal(respbody.Data, &blockData)
		if err != nil {
			fmt.Printf("[Error]Download file from CESS error:%s\n", err)
			logger.OutPutLogger.Sugar().Infof("[Error]Download file from CESS error:%s\n", err)
			return
		}

		_, err = installfile.Write(blockData.Data)
		if err != nil {
			fmt.Printf("%s[Error]:Failed to write file's block to file:%s%s error:%s\n", tools.Red, filepath.Join(conf.ClientConf.PathInfo.InstallPath, string(fileinfo.File_Name[:])), tools.Reset, err)
			logger.OutPutLogger.Sugar().Infof("%s[Error]:Failed to write file's block to file:%s%s error:%s", tools.Red, filepath.Join(conf.ClientConf.PathInfo.InstallPath, string(fileinfo.File_Name[:])), tools.Reset, err)
			return
		}

		getAllBar.Do(func() {
			bar.NewOption(0, int64(blockData.BlockNum))
		})
		bar.Play(int64(blockData.Blocks))
		wantfile.Blocks++
		sp.Put(req)
		if blockData.Blocks == blockData.BlockNum {
			break
		}
	}

	bar.Finish()
	fmt.Printf("%s[OK]:File '%s' has been downloaded to the directory :%s%s\n", tools.Green, string(fileinfo.File_Name), filepath.Join(conf.ClientConf.PathInfo.InstallPath, string(fileinfo.File_Name[:])), tools.Reset)
	//logger.OutPutLogger.Sugar().Infof("%s[OK]:File '%s' has been downloaded to the directory :%s%s", tools.Green,string(fileinfo.Filename),filepath.Join(conf.ClientConf.PathInfo.InstallPath,string(fileinfo.Filename[:])), tools.Reset)

	if !fileinfo.Public {
		fmt.Printf("%s[Warm]This is a private file, please enter the file password:%s\n", tools.Green, tools.Reset)
		fmt.Printf("Password:")
		filePWD := ""
		fmt.Scanln(&filePWD)
		encodefile, err := ioutil.ReadFile(filepath.Join(conf.ClientConf.PathInfo.InstallPath, string(fileinfo.File_Name[:])))
		if err != nil {
			fmt.Printf("%s[Error]:Decode file:%s fail%s error:%s\n", tools.Red, filepath.Join(conf.ClientConf.PathInfo.InstallPath, string(fileinfo.File_Name[:])), tools.Reset, err)
			logger.OutPutLogger.Sugar().Infof("%s[Error]:Decode file:%s fail%s error:%s\n", tools.Red, filepath.Join(conf.ClientConf.PathInfo.InstallPath, string(fileinfo.File_Name[:])), tools.Reset, err)
			return
		}
		decodefile, err := tools.AesDecrypt(encodefile, []byte(filePWD))
		if err != nil {
			fmt.Println("[Error]Dncode the file fail ,error! ", err)
			return
		}
		err = installfile.Truncate(0)
		_, err = installfile.Seek(0, os.SEEK_SET)
		_, err = installfile.Write(decodefile[:])
	}

	return
}

/*
FileDelete means to delete the file from the CESS system by the file id
fileid:fileid of the file that needs to be deleted
*/
func FileDelete(fileid string) {
	chain.Chain_Init()
	var ci chain.CessInfo
	ci.RpcAddr = conf.ClientConf.ChainData.CessRpcAddr
	ci.IdentifyAccountPhrase = conf.ClientConf.ChainData.IdAccountPhraseOrSeed
	ci.TransactionName = chain.DeleteFileTransactionName

	err := ci.DeleteFileOnChain(fileid)
	if err != nil {
		fmt.Printf("%s[Error]Delete file error:%s%s\n", tools.Red, tools.Reset, err)
		logger.OutPutLogger.Sugar().Infof("%s[Error]Delete file error:%s%s\n", tools.Red, tools.Reset, err)
		return
	} else {
		fmt.Printf("%s[OK]Delete fileid:%s success!%s\n", tools.Green, fileid, tools.Reset)
		logger.OutPutLogger.Sugar().Infof("%s[OK]Delete fileid:%s success!%s\n", tools.Green, fileid, tools.Reset)
		return
	}

}