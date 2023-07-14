import { BigNumber, Signer, Wallet, ethers } from 'ethers'
import {
  L1CustomGateway__factory,
  L1ERC20Gateway__factory,
  L1GatewayRouter__factory,
  L1AtomicTokenBridgeCreator__factory,
  L2AtomicTokenBridgeFactory__factory,
  L2GatewayRouter__factory,
  L2ERC20Gateway__factory,
  L2CustomGateway__factory,
  L1AtomicTokenBridgeCreator,
  L2WethGateway__factory,
  AeWETH__factory,
  L1WethGateway__factory,
  TransparentUpgradeableProxy__factory,
  ProxyAdmin,
  ProxyAdmin__factory,
} from '../build/types'
import { JsonRpcProvider } from '@ethersproject/providers'
import {
  L1ToL2MessageGasEstimator,
  L1ToL2MessageStatus,
  L1TransactionReceipt,
} from '@arbitrum/sdk'
import { exit } from 'process'
import { getBaseFee } from '@arbitrum/sdk/dist/lib/utils/lib'

/**
 * Use already deployed L1TokenBridgeCreator to create and init token bridge contracts.
 *
 * @param l1Signer
 * @param l2Signer
 * @param inboxAddress
 * @returns
 */
export const createTokenBridge = async (
  l1Signer: Signer,
  l2Provider: ethers.providers.Provider,
  l1TokenBridgeCreator: L1AtomicTokenBridgeCreator,
  inboxAddress: string
) => {
  const gasPrice = await l2Provider.getGasPrice()

  //// run retryable estimate for deploying L2 factory
  const deployFactoryGasParams = await getEstimateForDeployingFactory(
    l1Signer,
    l2Provider
  )
  const maxGasForFactory =
    await l1TokenBridgeCreator.gasLimitForL2FactoryDeployment()
  const maxSubmissionCostForFactory = deployFactoryGasParams.maxSubmissionCost

  console.log(
    'deployFactoryGasParams',
    maxGasForFactory.toString(),
    maxSubmissionCostForFactory.toString()
  )

  //// run retryable estimate for deploying L2 contracts
  //// we do this estimate using L2 factory template on L1 because on L2 factory does not yet exist
  const l2FactoryTemplate = L2AtomicTokenBridgeFactory__factory.connect(
    await l1TokenBridgeCreator.l2TokenBridgeFactoryTemplate(),
    l1Signer
  )
  const l2Code = {
    router: L2GatewayRouter__factory.bytecode,
    standardGateway: L2ERC20Gateway__factory.bytecode,
    customGateway: L2CustomGateway__factory.bytecode,
    wethGateway: L2WethGateway__factory.bytecode,
    aeWeth: AeWETH__factory.bytecode,
  }
  const gasEstimateToDeployContracts =
    await l2FactoryTemplate.estimateGas.deployL2Contracts(
      l2Code,
      ethers.Wallet.createRandom().address,
      ethers.Wallet.createRandom().address,
      ethers.Wallet.createRandom().address,
      ethers.Wallet.createRandom().address,
      ethers.Wallet.createRandom().address,
      ethers.Wallet.createRandom().address,
      ethers.Wallet.createRandom().address
    )
  const maxGasForContracts = gasEstimateToDeployContracts.mul(2)
  const maxSubmissionCostForContracts =
    deployFactoryGasParams.maxSubmissionCost.mul(2)

  let retryableValue = maxSubmissionCostForFactory
    .add(maxSubmissionCostForContracts)
    .add(maxGasForFactory.mul(gasPrice))
    .add(maxGasForContracts.mul(gasPrice))

  /// do it - create token bridge
  const receipt = await (
    await l1TokenBridgeCreator.createTokenBridge(
      inboxAddress,
      maxGasForContracts,
      gasPrice,
      { value: retryableValue }
    )
  ).wait()

  /// wait for execution of both tickets
  const l1TxReceipt = new L1TransactionReceipt(receipt)
  const messages = await l1TxReceipt.getL1ToL2Messages(l2Provider)
  const messageResults = await Promise.all(
    messages.map(message => message.waitForStatus())
  )

  // if both tickets are not redeemed log it and exit
  if (
    messageResults[0].status !== L1ToL2MessageStatus.REDEEMED ||
    messageResults[1].status !== L1ToL2MessageStatus.REDEEMED
  ) {
    console.log(
      `Retryable ticket (ID ${messages[0].retryableCreationId}) status: ${
        L1ToL2MessageStatus[messageResults[0].status]
      }`
    )
    console.log(
      `Retryable ticket (ID ${messages[1].retryableCreationId}) status: ${
        L1ToL2MessageStatus[messageResults[1].status]
      }`
    )
    exit()
  }

  /// pick up L2 factory address from 1st ticket
  const l2AtomicTokenBridgeFactory =
    L2AtomicTokenBridgeFactory__factory.connect(
      messageResults[0].l2TxReceipt.contractAddress,
      l2Provider
    )
  console.log('L2AtomicTokenBridgeFactory', l2AtomicTokenBridgeFactory.address)

  /// pick up L1 contracts from events
  const {
    router: l1Router,
    standardGateway: l1StandardGateway,
    customGateway: l1CustomGateway,
    wethGateway: l1WethGateway,
    proxyAdmin: l1ProxyAdmin,
  } = getParsedLogs(
    receipt.logs,
    l1TokenBridgeCreator.interface,
    'OrbitTokenBridgeCreated'
  )[0].args

  /// pick up L2 contracts
  const l2Router = await l1TokenBridgeCreator.getCanonicalL2RouterAddress()
  const l2StandardGateway = L2ERC20Gateway__factory.connect(
    await l1TokenBridgeCreator.getCanonicalL2StandardGatewayAddress(),
    l2Provider
  )
  const beaconProxyFactory = await l2StandardGateway.beaconProxyFactory()
  const l2CustomGateway =
    await l1TokenBridgeCreator.getCanonicalL2CustomGatewayAddress()
  const l2WethGateway = L2WethGateway__factory.connect(
    await l1TokenBridgeCreator.getCanonicalL2WethGatewayAddress(),
    l2Provider
  )
  const l1Weth = await l1TokenBridgeCreator.l1Weth()
  const l2Weth = await l1TokenBridgeCreator.getCanonicalL2WethAddress()
  const l2ProxyAdmin = await l1TokenBridgeCreator.canonicalL2ProxyAdminAddress()

  return {
    l1Router,
    l1StandardGateway,
    l1CustomGateway,
    l1WethGateway,
    l1ProxyAdmin,
    l2Router,
    l2StandardGateway: l2StandardGateway.address,
    l2CustomGateway,
    l2WethGateway: l2WethGateway.address,
    l1Weth,
    l2Weth,
    beaconProxyFactory,
    l2ProxyAdmin,
  }
}

/**
 * Deploy token bridge creator contract to base chain and set all the templates
 * @param l1Deployer
 * @param l2Provider
 * @param l1WethAddress
 * @returns
 */
export const deployL1TokenBridgeCreator = async (
  l1Deployer: Signer,
  l2Provider: ethers.providers.Provider,
  l1WethAddress: string
) => {
  /// deploy creator behind proxy
  const l1TokenBridgeCreatorProxyAdmin = await new ProxyAdmin__factory(
    l1Deployer
  ).deploy()
  await l1TokenBridgeCreatorProxyAdmin.deployed()

  const l1TokenBridgeCreatorLogic =
    await new L1AtomicTokenBridgeCreator__factory(l1Deployer).deploy()
  await l1TokenBridgeCreatorLogic.deployed()

  const l1TokenBridgeCreatorProxy =
    await new TransparentUpgradeableProxy__factory(l1Deployer).deploy(
      l1TokenBridgeCreatorLogic.address,
      l1TokenBridgeCreatorProxyAdmin.address,
      '0x'
    )
  await l1TokenBridgeCreatorProxy.deployed()

  const l1TokenBridgeCreator = L1AtomicTokenBridgeCreator__factory.connect(
    l1TokenBridgeCreatorProxy.address,
    l1Deployer
  )
  await (await l1TokenBridgeCreator.initialize()).wait()

  /// deploy L1 logic contracts
  const routerTemplate = await new L1GatewayRouter__factory(l1Deployer).deploy()
  await routerTemplate.deployed()

  const standardGatewayTemplate = await new L1ERC20Gateway__factory(
    l1Deployer
  ).deploy()
  await standardGatewayTemplate.deployed()

  const customGatewayTemplate = await new L1CustomGateway__factory(
    l1Deployer
  ).deploy()
  await customGatewayTemplate.deployed()

  const wethGatewayTemplate = await new L1WethGateway__factory(
    l1Deployer
  ).deploy()
  await wethGatewayTemplate.deployed()

  /// deploy L2 contracts as placeholders on L1

  const l2TokenBridgeFactoryOnL1 =
    await new L2AtomicTokenBridgeFactory__factory(l1Deployer).deploy()
  await l2TokenBridgeFactoryOnL1.deployed()

  const l2GatewayRouterOnL1 = await new L2GatewayRouter__factory(
    l1Deployer
  ).deploy()
  await l2GatewayRouterOnL1.deployed()

  const l2StandardGatewayAddressOnL1 = await new L2ERC20Gateway__factory(
    l1Deployer
  ).deploy()
  await l2StandardGatewayAddressOnL1.deployed()

  const l2CustomGatewayAddressOnL1 = await new L2CustomGateway__factory(
    l1Deployer
  ).deploy()
  await l2CustomGatewayAddressOnL1.deployed()

  const l2WethGatewayAddressOnL1 = await new L2WethGateway__factory(
    l1Deployer
  ).deploy()
  await l2WethGatewayAddressOnL1.deployed()

  const l2WethAddressOnL1 = await new AeWETH__factory(l1Deployer).deploy()
  await l2WethAddressOnL1.deployed()

  //// run retryable estimate for deploying L2 factory
  const deployFactoryGasParams = await getEstimateForDeployingFactory(
    l1Deployer,
    l2Provider
  )

  await (
    await l1TokenBridgeCreator.setTemplates(
      routerTemplate.address,
      standardGatewayTemplate.address,
      customGatewayTemplate.address,
      wethGatewayTemplate.address,
      l2TokenBridgeFactoryOnL1.address,
      l2GatewayRouterOnL1.address,
      l2StandardGatewayAddressOnL1.address,
      l2CustomGatewayAddressOnL1.address,
      l2WethGatewayAddressOnL1.address,
      l2WethAddressOnL1.address,
      l1WethAddress,
      deployFactoryGasParams.gasLimit
    )
  ).wait()

  return l1TokenBridgeCreator
}

export const getEstimateForDeployingFactory = async (
  l1Deployer: Signer,
  l2Provider: ethers.providers.Provider
) => {
  //// run retryable estimate for deploying L2 factory
  const l1DeployerAddress = await l1Deployer.getAddress()
  const l1ToL2MsgGasEstimate = new L1ToL2MessageGasEstimator(l2Provider)
  const deployFactoryGasParams = await l1ToL2MsgGasEstimate.estimateAll(
    {
      from: ethers.Wallet.createRandom().address,
      to: ethers.constants.AddressZero,
      l2CallValue: BigNumber.from(0),
      excessFeeRefundAddress: l1DeployerAddress,
      callValueRefundAddress: l1DeployerAddress,
      data: L2AtomicTokenBridgeFactory__factory.bytecode,
    },
    await getBaseFee(l1Deployer.provider!),
    l1Deployer.provider!
  )

  return deployFactoryGasParams
}

export const getSigner = (provider: JsonRpcProvider, key?: string) => {
  if (!key && !provider)
    throw new Error('Provide at least one of key or provider.')
  if (key) return new Wallet(key).connect(provider)
  else return provider.getSigner(0)
}

export const getParsedLogs = (
  logs: ethers.providers.Log[],
  iface: ethers.utils.Interface,
  eventName: string
) => {
  const eventFragment = iface.getEvent(eventName)
  const parsedLogs = logs
    .filter(
      (curr: any) => curr.topics[0] === iface.getEventTopic(eventFragment)
    )
    .map((curr: any) => iface.parseLog(curr))
  return parsedLogs
}

export function sleep(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms))
}
