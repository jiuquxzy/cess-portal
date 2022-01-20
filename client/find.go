package client

import (
	"dapp_cess_client/conf"
	"dapp_cess_client/internal/chain"
	"dapp_cess_client/internal/logger"
	"fmt"
)

func FindPurchasedSpace() {
	chain.Chain_Init()

	var ci chain.CessInfo
	ci.RpcAddr = conf.ClientConf.ChainData.CessRpcAddr
	ci.ChainModule = chain.PurchasedSpaceChainModule
	ci.ChainModuleMethod = chain.PurchasedSpaceModuleMethod

	userinfo, err := ci.UserHoldSpaceDetails()
	if err != nil {
		fmt.Printf("[Error]Get user data fail:%s\n", err)
		logger.OutPutLogger.Sugar().Infof("[Error]Get user data fail:%s\n", err)
		return
	}
	fmt.Println(userinfo)
}

func FindPrice() {
	chain.Chain_Init()

	var ci chain.CessInfo
	ci.RpcAddr = conf.ClientConf.ChainData.CessRpcAddr
	ci.ChainModule = chain.FindPriceChainModule

	ci.ChainModuleMethod = chain.FindPriceModuleMethod[0]
	AllPurchased, err := ci.GetPurchasedSpace()
	if err != nil {
		fmt.Printf("[Error]Get all purchased fail:%s\n", err)
		logger.OutPutLogger.Sugar().Infof("[Error]Get all purchased fail::%s\n", err)
		return
	}

	ci.ChainModuleMethod = chain.FindPriceModuleMethod[1]
	AllAvailable, err := ci.GetAvailableSpace()
	if err != nil {
		fmt.Printf("[Error]Get all available fail:%s\n", err)
		logger.OutPutLogger.Sugar().Infof("[Error]Get all available fail::%s\n", err)
		return
	}

	purc := AllPurchased.Int64()
	ava := AllAvailable.Int64()

	result := float64((ava - purc)) / 1024 * 1000

	fmt.Printf("[successful]The current storage price is:%f per (MB)\n", result)
	logger.OutPutLogger.Sugar().Infof("[successful]The current storage price is:%f per (MB)\n", result)

}

func FindFile(fileid string) {
	chain.Chain_Init()

	var ci chain.CessInfo
	ci.RpcAddr = conf.ClientConf.ChainData.CessRpcAddr
	ci.ChainModule = chain.FindFileChainModule

	if fileid != "" {
		ci.ChainModuleMethod = chain.FindFileModuleMethod[0]
		data, err := ci.GetFileInfo(fileid)
		if err != nil {
			fmt.Printf("[Error]Get file:%s info fail:%s\n", fileid, err)
			logger.OutPutLogger.Sugar().Infof("[Error]Get file:%s info fail:%s\n", fileid, err)
			return
		}
		fmt.Println(data)
	} else {
		ci.ChainModuleMethod = chain.FindFileModuleMethod[1]
		data, err := ci.GetFileList()
		if err != nil {
			fmt.Printf("[Error]Get file list fail:%s\n", err)
			logger.OutPutLogger.Sugar().Infof("[Error]Get file list fail:%s\n", err)
			return
		}
		for _, fileinfo := range data {
			fmt.Printf("%s\n", string(fileinfo))
		}
	}
}
