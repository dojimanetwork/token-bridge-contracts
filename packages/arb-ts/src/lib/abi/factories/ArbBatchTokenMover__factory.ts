/* Autogenerated file. Do not edit manually. */
/* tslint:disable */
/* eslint-disable */

import { Signer } from 'ethers'
import { Provider, TransactionRequest } from '@ethersproject/providers'
import { Contract, ContractFactory, Overrides } from '@ethersproject/contracts'

import type { ArbBatchTokenMover } from '../ArbBatchTokenMover'

export class ArbBatchTokenMover__factory extends ContractFactory {
  constructor(
    linkLibraryAddresses: ArbBatchTokenMoverLibraryAddresses,
    signer?: Signer
  ) {
    super(
      _abi,
      ArbBatchTokenMover__factory.linkBytecode(linkLibraryAddresses),
      signer
    )
  }

  static linkBytecode(
    linkLibraryAddresses: ArbBatchTokenMoverLibraryAddresses
  ): string {
    let linkedBytecode = _bytecode

    linkedBytecode = linkedBytecode.replace(
      new RegExp('__\\$6bf011c07207fd4b4f621f21ef466e3cdb\\$__', 'g'),
      linkLibraryAddresses['__$6bf011c07207fd4b4f621f21ef466e3cdb$__']
        .replace(/^0x/, '')
        .toLowerCase()
    )

    return linkedBytecode
  }

  deploy(overrides?: Overrides): Promise<ArbBatchTokenMover> {
    return super.deploy(overrides || {}) as Promise<ArbBatchTokenMover>
  }
  getDeployTransaction(overrides?: Overrides): TransactionRequest {
    return super.getDeployTransaction(overrides || {})
  }
  attach(address: string): ArbBatchTokenMover {
    return super.attach(address) as ArbBatchTokenMover
  }
  connect(signer: Signer): ArbBatchTokenMover__factory {
    return super.connect(signer) as ArbBatchTokenMover__factory
  }
  static connect(
    address: string,
    signerOrProvider: Signer | Provider
  ): ArbBatchTokenMover {
    return new Contract(address, _abi, signerOrProvider) as ArbBatchTokenMover
  }
}

const _abi = [
  {
    inputs: [],
    name: 'exitToL1',
    outputs: [],
    stateMutability: 'nonpayable',
    type: 'function',
  },
  {
    inputs: [
      {
        internalType: 'uint256',
        name: 'amount',
        type: 'uint256',
      },
    ],
    name: 'withdrawInBatch',
    outputs: [],
    stateMutability: 'nonpayable',
    type: 'function',
  },
]

const _bytecode =
  '0x608060405234801561001057600080fd5b506106a4806100206000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c8063733490d01461003b578063d5d683a414610045575b600080fd5b610043610062565b005b6100436004803603602081101561005b57600080fd5b50356104af565b6005544310156100a4576040805162461bcd60e51b81526020600482015260086024820152672a27a7afa9a7a7a760c11b604482015290519081900360640190fd5b6006546040805163c2eeeebd60e01b815290516000926001600160a01b03169163c2eeeebd916004808301926020929190829003018186803b1580156100e957600080fd5b505afa1580156100fd573d6000803e3d6000fd5b505050506040513d602081101561011357600080fd5b505160408051633148b9ef60e21b815260006004820152905191925060649163928c169a91309173__$6bf011c07207fd4b4f621f21ef466e3cdb$__9163c522e7bc916024808301926020929190829003018186803b15801561017557600080fd5b505af4158015610189573d6000803e3d6000fd5b505050506040513d602081101561019f57600080fd5b5051604080516024808201939093526001600160a01b03808816604480840191909152835180840382018152606493840185526020810180516001600160e01b0316630c9fb1e560e11b178152855160e08a901b6001600160e01b031916815293881660048501908152968401958652815192840192909252805190959493929092019180838360005b83811015610241578181015183820152602001610229565b50505050905090810190601f16801561026e5780820380516001836020036101000a031916815260200191505b509350505050602060405180830381600087803b15801561028e57600080fd5b505af11580156102a2573d6000803e3d6000fd5b505050506040513d60208110156102b857600080fd5b5050600754600654604080516370a0823160e01b8152306004820181905291516001600160a01b039485169463d2ce7d65948794939116916370a0823191602480820192602092909190829003018186803b15801561031657600080fd5b505afa15801561032a573d6000803e3d6000fd5b505050506040513d602081101561034057600080fd5b5051604080516001600160e01b031960e087901b1681526001600160a01b03948516600482015292909316602483015260448201526000606482018190526084820181905260c060a4830152600260c483015261060f60f31b60e48301529151610104808301939282900301818387803b1580156103bd57600080fd5b505af11580156103d1573d6000803e3d6000fd5b505050506040513d6000823e601f3d908101601f1916820160405260208110156103fa57600080fd5b810190808051604051939291908464010000000082111561041a57600080fd5b90830190602082018581111561042f57600080fd5b825164010000000081118282018810171561044957600080fd5b82525081516020918201929091019080838360005b8381101561047657818101518382015260200161045e565b50505050905090810190601f1680156104a35780820380516001836020036101000a031916815260200191505b50604052503392505050ff5b600654604080516323b872dd60e01b81523360048201523060248201526044810184905290516001600160a01b03909216916323b872dd916064808201926020929091908290030181600087803b15801561050957600080fd5b505af115801561051d573d6000803e3d6000fd5b505050506040513d602081101561053357600080fd5b5051610578576040805162461bcd60e51b815260206004820152600f60248201526e1514905394d1915497d19052531151608a1b604482015290519081900360640190fd5b604080513360208083019190915281830184905282518083038401815260608301808552637319a76f60e11b905260006064840181815260848501958652825160a4860152825173__$6bf011c07207fd4b4f621f21ef466e3cdb$__9663e6334ede9693959293909260c49091019190850190808383895b838110156106085781810151838201526020016105f0565b50505050905090810190601f1680156106355780820380516001836020036101000a031916815260200191505b50935050505060006040518083038186803b15801561065357600080fd5b505af4158015610667573d6000803e3d6000fd5b505050505056fea26469706673582212209c4b98aee2007d8c930acafa5873305d35341aaaff10a3337b25b5eca591ca7f64736f6c634300060b0033'

export interface ArbBatchTokenMoverLibraryAddresses {
  ['__$6bf011c07207fd4b4f621f21ef466e3cdb$__']: string
}
